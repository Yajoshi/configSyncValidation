package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"configsync-validator/model"
	"configsync-validator/monitor"
)

// Getter implements monitor.Getter on top of a dynamic client.
type Getter struct {
	Client dynamic.Interface
}

var _ monitor.Getter = (*Getter)(nil)

// GetSync returns the RootSync or RepoSync object.
func (g *Getter) GetSync(ctx context.Context, t model.SyncTarget) (map[string]interface{}, error) {
	obj, err := g.Client.Resource(SyncGVR(t)).Namespace(t.Namespace).Get(ctx, t.Name, metav1.GetOptions{})
	if err != nil {
		return nil, translate(err)
	}
	return obj.Object, nil
}

// GetResourceGroup returns the ResourceGroup that tracks the sync. When it is
// missing, the error lists the ResourceGroups that do exist in the namespace so
// a wrong name is easy to spot.
func (g *Getter) GetResourceGroup(ctx context.Context, t model.SyncTarget) (map[string]interface{}, error) {
	rgs := g.Client.Resource(ResourceGroupGVR).Namespace(t.Namespace)
	obj, err := rgs.Get(ctx, t.RGName(), metav1.GetOptions{})
	if err == nil {
		return obj.Object, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	hint := ""
	if list, listErr := rgs.List(ctx, metav1.ListOptions{}); listErr == nil && len(list.Items) > 0 {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.GetName())
		}
		sort.Strings(names)
		hint = fmt.Sprintf(" (ResourceGroups in %s: %s; set resourceGroupName if one of them belongs to this sync)",
			t.Namespace, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("%w: ResourceGroup %s/%s%s", monitor.ErrNotFound, t.Namespace, t.RGName(), hint)
}

func translate(err error) error {
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: %v", monitor.ErrNotFound, err)
	}
	return err
}
