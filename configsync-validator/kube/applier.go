package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	"configsync-validator/model"
	"configsync-validator/syncer"
)

// Applier implements syncer.Applier using server-side apply.
type Applier struct {
	Client       dynamic.Interface
	FieldManager string
	Force        bool
}

var _ syncer.Applier = (*Applier)(nil)

// LoadManifest reads a YAML or JSON file that must contain exactly one object.
func (a *Applier) LoadManifest(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := utilyaml.NewYAMLOrJSONDecoder(f, 4096)
	var found map[string]interface{}
	for {
		var raw map[string]interface{}
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse: %w", err)
		}
		if len(raw) == 0 {
			continue // empty document, for example a trailing "---"
		}
		if found != nil {
			return nil, errors.New("file contains more than one document; expected exactly one")
		}
		// Round-trip through Unstructured so numbers are normalised to int64.
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(data); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		found = u.Object
	}
	if found == nil {
		return nil, errors.New("file contains no object")
	}
	return found, nil
}

// Apply creates or updates the object with server-side apply.
func (a *Applier) Apply(ctx context.Context, obj map[string]interface{}, t model.SyncTarget) error {
	u := (&unstructured.Unstructured{Object: obj}).DeepCopy()
	u.SetNamespace(t.Namespace)
	u.SetResourceVersion("")
	u.SetManagedFields(nil)
	unstructured.RemoveNestedField(u.Object, "status")

	data, err := u.MarshalJSON()
	if err != nil {
		return fmt.Errorf("encode %s: %w", t, err)
	}

	manager := a.FieldManager
	if manager == "" {
		manager = "configsync-validator"
	}
	force := a.Force

	_, err = a.Client.Resource(SyncGVR(t)).Namespace(t.Namespace).Patch(
		ctx, t.Name, types.ApplyPatchType, data,
		metav1.PatchOptions{FieldManager: manager, Force: &force},
	)
	return err
}
