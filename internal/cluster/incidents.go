package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// GetIncident performs one exact-key canonical search. It deliberately has no
// unfiltered compatibility fallback because that could return the wrong
// incident when a server does not support the requested filter.
func (c *Client) GetIncident(ctx context.Context, incidentKey string) (Incident, bool, error) {
	if incidentKey == "" {
		return Incident{}, false, errors.New("incident key is required")
	}
	result, err := c.SearchIncidentsInventory(ctx, map[string]any{
		"incidentKey": incidentKey,
	}, InventoryLimits{
		PageSize: 2, MaxPages: 1, MaxItems: 2, MaxBodyBytes: defaultInventoryBodySize,
	})
	if err != nil {
		return Incident{}, false, err
	}
	if result.Partial {
		message := "exact incident search was partial"
		if len(result.Warnings) > 0 {
			message += ": " + result.Warnings[0].Message
		}
		return Incident{}, false, errors.New(message)
	}
	var found *Incident
	for index := range result.Items {
		item := result.Items[index]
		if item.Key != incidentKey {
			return Incident{}, false, fmt.Errorf(
				"incident search returned mismatched incident key %q for exact key %q",
				item.Key, incidentKey,
			)
		}
		if found != nil {
			return Incident{}, false, fmt.Errorf("incident search returned duplicate exact key %q", incidentKey)
		}
		found = &item
	}
	if found == nil {
		return Incident{}, false, nil
	}
	return *found, true, nil
}

// SearchProcessDefinitionsInventory is the canonical bounded definition
// summary search used for incident detail enrichment.
func (c *Client) SearchProcessDefinitionsInventory(
	ctx context.Context,
	filter map[string]any,
	limits InventoryLimits,
) (SearchResult[ProcessDefinition], error) {
	return searchTyped(ctx, c, "/process-definitions/search", filter, limits, "process-definitions",
		func(raw json.RawMessage) (ProcessDefinition, string, error) {
			var wire struct {
				Key          identifier  `json:"processDefinitionKey"`
				ID           string      `json:"processDefinitionId"`
				Name         *string     `json:"name"`
				Version      int         `json:"version"`
				VersionTag   *string     `json:"versionTag"`
				ResourceName string      `json:"resourceName"`
				TenantID     string      `json:"tenantId"`
				HasStartForm *bool       `json:"hasStartForm"`
				StartFormKey *identifier `json:"startFormKey"`
			}
			if err := decodeStrictJSON(raw, &wire); err != nil {
				return ProcessDefinition{}, "", err
			}
			if wire.Key == "" || strings.TrimSpace(wire.ID) == "" || wire.Version < 1 {
				return ProcessDefinition{}, "", errors.New("process definition requires key, ID, and positive version")
			}
			item := ProcessDefinition{
				Key: string(wire.Key), ProcessDefinitionID: wire.ID, Name: wire.Name,
				Version: wire.Version, VersionTag: wire.VersionTag,
				ResourceName: wire.ResourceName, TenantID: wire.TenantID,
				HasStartForm: wire.HasStartForm,
			}
			if wire.StartFormKey != nil {
				key := string(*wire.StartFormKey)
				item.StartFormKey = &key
			}
			return item, item.Key, nil
		})
}
