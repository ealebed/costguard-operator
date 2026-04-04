package controller

import (
	"testing"

	finopsv1alpha1 "github.com/ealebed/costguard-operator/api/v1alpha1"
)

func TestSpendAtOrOverMax(t *testing.T) {
	t.Parallel()
	if !spendAtOrOverMax(1.0, 1.0) {
		t.Fatal("1.0 should be at or over 1.0")
	}
	if spendAtOrOverMax(0.99, 1.0) {
		t.Fatal("0.99 should be below 1.0")
	}
}

func TestValidateCostBudgetSpec(t *testing.T) {
	t.Parallel()
	spec := finopsv1alpha1.BudgetNamespaceSpec{
		CostBudget: &finopsv1alpha1.BudgetNamespaceCostBudgetSpec{
			Enabled:            true,
			BillingExportTable: "p.d.t",
			ClusterName:        "c",
			MaxSpendUSD:        "1",
		},
	}
	if err := validateCostBudgetSpec(&spec); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	spec.CostBudget.MaxSpendUSD = "0"
	if err := validateCostBudgetSpec(&spec); err == nil {
		t.Fatal("max 0 should fail")
	}
}

func TestEnforcementScaleReason(t *testing.T) {
	t.Parallel()
	if enforcementScaleReason(true, false) != "resource-quota" {
		t.Fatal()
	}
	if enforcementScaleReason(false, true) != "cost-budget" {
		t.Fatal()
	}
	if enforcementScaleReason(true, true) != "resource-quota-and-cost-budget" {
		t.Fatal()
	}
}
