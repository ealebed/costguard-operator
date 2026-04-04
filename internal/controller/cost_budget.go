package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

const (
	defaultCostBudgetLookback      = 168 * time.Hour
	defaultCostBudgetQueryInterval = time.Hour
	costBudgetEpsilon              = 1e-9
)

// NamespaceSpendQuerier fetches billed USD for a namespace from BigQuery billing export.
type NamespaceSpendQuerier interface {
	NamespaceSpendUSD(ctx context.Context, tableRef, clusterName, namespace string, lookback time.Duration) (float64, error)
}

func costBudgetEnabled(spec *finopsv1alpha1.BudgetNamespaceSpec) bool {
	cb := spec.CostBudget
	return cb != nil && cb.Enabled
}

func validateCostBudgetSpec(spec *finopsv1alpha1.BudgetNamespaceSpec) error {
	cb := spec.CostBudget
	if cb == nil || !cb.Enabled {
		return nil
	}
	if strings.TrimSpace(cb.BillingExportTable) == "" {
		return fmt.Errorf("spec.costBudget.billingExportTable is required when cost budget is enabled")
	}
	if strings.TrimSpace(cb.ClusterName) == "" {
		return fmt.Errorf("spec.costBudget.clusterName is required when cost budget is enabled")
	}
	if strings.TrimSpace(cb.MaxSpendUSD) == "" {
		return fmt.Errorf("spec.costBudget.maxSpendUSD is required when cost budget is enabled")
	}
	maxUSD, err := strconv.ParseFloat(strings.TrimSpace(cb.MaxSpendUSD), 64)
	if err != nil {
		return fmt.Errorf("parse spec.costBudget.maxSpendUSD: %w", err)
	}
	if maxUSD <= 0 {
		return fmt.Errorf("spec.costBudget.maxSpendUSD must be greater than zero")
	}
	if w := strings.TrimSpace(cb.LookbackWindow); w != "" {
		if _, err := time.ParseDuration(w); err != nil {
			return fmt.Errorf("parse spec.costBudget.lookbackWindow: %w", err)
		}
	}
	if w := strings.TrimSpace(cb.QueryInterval); w != "" {
		if _, err := time.ParseDuration(w); err != nil {
			return fmt.Errorf("parse spec.costBudget.queryInterval: %w", err)
		}
	}
	return nil
}

func costBudgetLookback(cb *finopsv1alpha1.BudgetNamespaceCostBudgetSpec) time.Duration {
	if cb == nil {
		return defaultCostBudgetLookback
	}
	w := strings.TrimSpace(cb.LookbackWindow)
	if w == "" {
		return defaultCostBudgetLookback
	}
	d, err := time.ParseDuration(w)
	if err != nil {
		return defaultCostBudgetLookback
	}
	return d
}

func costBudgetQueryInterval(cb *finopsv1alpha1.BudgetNamespaceCostBudgetSpec) time.Duration {
	if cb == nil {
		return defaultCostBudgetQueryInterval
	}
	w := strings.TrimSpace(cb.QueryInterval)
	if w == "" {
		return defaultCostBudgetQueryInterval
	}
	d, err := time.ParseDuration(w)
	if err != nil {
		return defaultCostBudgetQueryInterval
	}
	return d
}

func costBudgetMaxUSD(cb *finopsv1alpha1.BudgetNamespaceCostBudgetSpec) (float64, error) {
	if cb == nil {
		return 0, fmt.Errorf("cost budget spec is nil")
	}
	return strconv.ParseFloat(strings.TrimSpace(cb.MaxSpendUSD), 64)
}

func spendAtOrOverMax(spend, maxUSD float64) bool {
	return spend+costBudgetEpsilon >= maxUSD
}

// resolveNamespaceSpendUSD returns observed spend for threshold checks. When throttled, it reuses status.lastObservedSpendUSD
// or runs a live query if no cache exists. On success after a live query, it updates bn.Status last query fields.
func (r *BudgetNamespaceReconciler) resolveNamespaceSpendUSD(ctx context.Context, bn *finopsv1alpha1.BudgetNamespace, now time.Time) (spend float64, err error) {
	if !costBudgetEnabled(&bn.Spec) {
		return 0, nil
	}
	if r.SpendQuerier == nil {
		return 0, fmt.Errorf("cost budget is enabled but BigQuery client is not configured on the operator")
	}
	cb := bn.Spec.CostBudget
	if err := validateCostBudgetSpec(&bn.Spec); err != nil {
		return 0, err
	}

	interval := costBudgetQueryInterval(cb)
	if bn.Status.LastCostQueryAt != nil && now.Before(bn.Status.LastCostQueryAt.Add(interval)) {
		if s := strings.TrimSpace(bn.Status.LastObservedSpendUSD); s != "" {
			return strconv.ParseFloat(s, 64)
		}
	}

	lookback := costBudgetLookback(cb)
	spend, err = r.SpendQuerier.NamespaceSpendUSD(ctx, cb.BillingExportTable, cb.ClusterName, bn.Spec.NamespaceName, lookback)
	if err != nil {
		return 0, err
	}
	t := metav1.NewTime(now)
	bn.Status.LastCostQueryAt = &t
	bn.Status.LastObservedSpendUSD = strconv.FormatFloat(spend, 'f', -1, 64)
	return spend, nil
}

func withinCostBudgetForRestore(spec *finopsv1alpha1.BudgetNamespaceSpec, spend float64) (bool, error) {
	if !costBudgetEnabled(spec) {
		return true, nil
	}
	maxUSD, err := costBudgetMaxUSD(spec.CostBudget)
	if err != nil {
		return false, err
	}
	return spend+costBudgetEpsilon < maxUSD, nil
}

func enforcementScaleReason(quotaAtHardLimit, costOverBudget bool) string {
	switch {
	case quotaAtHardLimit && costOverBudget:
		return "resource-quota-and-cost-budget"
	case costOverBudget:
		return "cost-budget"
	default:
		return "resource-quota"
	}
}

func overBudgetReasonMessage(quotaAtHardLimit, costOverBudget bool) (reason, message string) {
	switch {
	case quotaAtHardLimit && costOverBudget:
		return "CostBudgetAndQuotaExceeded", "ResourceQuota is at or above a hard limit and billed spend is at or above maxSpendUSD; " +
			"non-exempt Deployments and StatefulSets were scaled to zero"
	case costOverBudget:
		return "CostBudgetExceeded", "Billed spend in the lookback window is at or above maxSpendUSD; non-exempt Deployments and StatefulSets were scaled to zero"
	default:
		return "ResourceQuotaAtOrOverHard", "ResourceQuota usage is at or above a hard limit; non-exempt Deployments and StatefulSets were scaled to zero"
	}
}
