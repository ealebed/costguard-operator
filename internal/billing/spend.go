// Package billing queries GCP BigQuery billing export for namespace spend.
package billing

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

var bqTablePart = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// SpendQuerier runs parameterized queries against a detailed billing export table.
type SpendQuerier struct {
	client *bigquery.Client
}

// NewSpendQuerier returns a querier that uses c. The caller owns the client lifecycle.
func NewSpendQuerier(c *bigquery.Client) *SpendQuerier {
	return &SpendQuerier{client: c}
}

func validateTableRef(tableRef string) (project, dataset, table string, err error) {
	parts := strings.Split(strings.TrimSpace(tableRef), ".")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("billing table must be project.dataset.table, got %q", tableRef)
	}
	for i, p := range parts {
		if !bqTablePart.MatchString(p) {
			return "", "", "", fmt.Errorf("invalid billing table segment %q (index %d)", p, i)
		}
	}
	return parts[0], parts[1], parts[2], nil
}

// NamespaceSpendUSD returns SUM(cost) in USD for rows labeled with the cluster and namespace.
func (q *SpendQuerier) NamespaceSpendUSD(ctx context.Context, tableRef, clusterName, namespace string, lookback time.Duration) (float64, error) {
	if q == nil || q.client == nil {
		return 0, fmt.Errorf("bigquery client is not configured")
	}
	proj, ds, tbl, err := validateTableRef(tableRef)
	if err != nil {
		return 0, err
	}
	if clusterName == "" || namespace == "" {
		return 0, fmt.Errorf("clusterName and namespace must be set")
	}
	start := time.Now().UTC().Add(-lookback)
	fq := fmt.Sprintf("`%s.%s.%s`", proj, ds, tbl)
	qy := q.client.Query(fmt.Sprintf(`
SELECT COALESCE(SUM(t.cost), 0) AS total_cost
FROM %s AS t
WHERE t.usage_start_time >= @start_ts
  AND EXISTS (SELECT 1 FROM UNNEST(t.labels) AS l WHERE l.key = 'goog-k8s-cluster-name' AND l.value = @cluster)
  AND EXISTS (SELECT 1 FROM UNNEST(t.labels) AS l WHERE l.key = 'goog-k8s-namespace' AND l.value = @namespace)
`, fq))
	qy.Parameters = []bigquery.QueryParameter{
		{Name: "start_ts", Value: start},
		{Name: "cluster", Value: clusterName},
		{Name: "namespace", Value: namespace},
	}

	it, err := qy.Read(ctx)
	if err != nil {
		return 0, fmt.Errorf("bigquery read: %w", err)
	}
	var row struct {
		TotalCost float64 `bigquery:"total_cost"`
	}
	err = it.Next(&row)
	if err == iterator.Done {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("bigquery row: %w", err)
	}
	return row.TotalCost, nil
}
