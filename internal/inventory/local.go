package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/project"
)

// LocalRequest selects a project using the same project discovery rules as the
// rest of the toolkit.
type LocalRequest struct {
	Root string
}

// BuildLocal recursively inventories configured project resource paths.
func BuildLocal(request LocalRequest) (Inventory, error) {
	opened, err := project.Open(request.Root)
	if err != nil {
		return Inventory{}, fmt.Errorf("open project inventory: %w", err)
	}
	localSource := Source{Type: "local", ProjectRoot: opened.Root}
	result := Inventory{Source: localSource}
	seen := make(map[string]string)
	for _, configured := range []struct {
		kind  Kind
		asset project.AssetKind
	}{
		{kind: KindProcess, asset: project.AssetBPMN},
		{kind: KindDecision, asset: project.AssetDMN},
		{kind: KindForm, asset: project.AssetForms},
	} {
		files, err := opened.Discover(configured.asset)
		if err != nil {
			return Inventory{}, fmt.Errorf("discover %s inventory: %w", configured.kind, err)
		}
		for _, path := range files {
			resources, err := localFileResources(opened.Root, configured.kind, path)
			if err != nil {
				return Inventory{}, fmt.Errorf("inventory %s: %w", relativePath(opened.Root, path), err)
			}
			for _, resource := range resources {
				resource.Source = localSource
				identity := resource.Kind.String() + "\x00" + resource.ID
				if prior, duplicate := seen[identity]; duplicate {
					return Inventory{}, fmt.Errorf("duplicate %s resource ID %q in %s and %s",
						resource.Kind, resource.ID, prior, resource.Path)
				}
				seen[identity] = resource.Path
				result.Resources = append(result.Resources, resource)
			}
		}
	}
	sortInventory(&result)
	if err := result.ValidateComparable(); err != nil {
		return Inventory{}, err
	}
	return result, nil
}

func localFileResources(root string, kind Kind, path string) ([]Resource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	relative := relativePath(root, path)
	if kind == KindProcess {
		doc, err := bpmn.Parse(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		ids, err := ResourceIDs(kind, raw)
		if err != nil {
			return nil, err
		}
		processes := make(map[string]bpmn.Process, len(doc.Processes))
		for _, process := range doc.Processes {
			processes[process.ID] = process
		}
		out := make([]Resource, 0, len(ids))
		for _, id := range ids {
			canonical, err := CanonicalizeProcess(raw, id)
			if err != nil {
				return nil, err
			}
			out = append(out, Resource{
				Kind: kind, ID: id, Name: processes[id].Name, Path: relative,
				Digest: digest(canonical), Source: Source{Type: "local"},
			})
		}
		return out, nil
	}
	ids, err := ResourceIDs(kind, raw)
	if err != nil {
		return nil, err
	}
	canonical, err := Canonicalize(kind, raw)
	if err != nil {
		return nil, err
	}
	out := make([]Resource, 0, len(ids))
	for _, id := range ids {
		out = append(out, Resource{
			Kind: kind, ID: id, Path: relative, Digest: digest(canonical),
			Source: Source{Type: "local"},
		})
	}
	return out, nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func relativePath(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(value)
}
