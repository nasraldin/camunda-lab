package cluster

import (
	"context"
	"errors"
	"fmt"
)

// GetProcessInstanceExact performs one exact-key canonical search. It has no
// GET-by-path fallback so callers stay on the shared inventory search contract.
func (c *Client) GetProcessInstanceExact(ctx context.Context, processInstanceKey string) (ProcessInstance, bool, error) {
	if processInstanceKey == "" {
		return ProcessInstance{}, false, errors.New("process instance key is required")
	}
	result, err := c.SearchProcessInstancesInventory(ctx, map[string]any{
		"processInstanceKey": processInstanceKey,
	}, InventoryLimits{
		PageSize: 2, MaxPages: 1, MaxItems: 2, MaxBodyBytes: defaultInventoryBodySize,
	})
	if err != nil {
		return ProcessInstance{}, false, err
	}
	if result.Partial {
		message := "exact process instance search was partial"
		if len(result.Warnings) > 0 {
			message += ": " + result.Warnings[0].Message
		}
		return ProcessInstance{}, false, errors.New(message)
	}
	var found *ProcessInstance
	for index := range result.Items {
		item := result.Items[index]
		if item.Key != processInstanceKey {
			return ProcessInstance{}, false, fmt.Errorf(
				"process instance search returned mismatched key %q for exact key %q",
				item.Key, processInstanceKey,
			)
		}
		if found != nil {
			return ProcessInstance{}, false, fmt.Errorf("process instance search returned duplicate exact key %q", processInstanceKey)
		}
		found = &item
	}
	if found == nil {
		return ProcessInstance{}, false, nil
	}
	return *found, true, nil
}
