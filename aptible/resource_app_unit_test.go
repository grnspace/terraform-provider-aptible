package aptible

import (
	"context"
	"strings"
	"testing"

	"github.com/aptible/aptible-api-go/aptibleapi"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// appStateBeforePreserve returns state for an app with one cmd service as a
// provider without preserve_autoscaled_container_count wrote it.
func appStateBeforePreserve(t *testing.T) *terraform.InstanceState {
	t.Helper()
	d := resourceApp().TestResourceData()
	d.SetId("1")
	_ = d.Set("app_id", 1)
	_ = d.Set("env_id", 1)
	_ = d.Set("handle", "app")
	if err := d.Set("service", []map[string]interface{}{{
		"process_type":           "cmd",
		"container_count":        2,
		"container_memory_limit": 1024,
		"container_profile":      "m",
		"stop_timeout":           10,
	}}); err != nil {
		t.Fatal(err)
	}
	s := d.State()
	for k := range s.Attributes {
		if strings.HasSuffix(k, ".preserve_autoscaled_container_count") {
			delete(s.Attributes, k)
		}
	}
	return s
}

func appConfig(service map[string]interface{}) *terraform.ResourceConfig {
	return terraform.NewResourceConfigRaw(map[string]interface{}{
		"env_id":  1,
		"handle":  "app",
		"service": []interface{}{service},
	})
}

func TestResourceAppUpgradeToPreserveAutoscaledContainerCount(t *testing.T) {
	tests := []struct {
		name       string
		service    map[string]interface{}
		wantChange bool
	}{
		{
			name:       "unchanged config plans no change",
			service:    map[string]interface{}{"process_type": "cmd", "container_count": 2},
			wantChange: false,
		},
		{
			name:       "enabling the flag plans a service change",
			service:    map[string]interface{}{"process_type": "cmd", "container_count": 2, "preserve_autoscaled_container_count": true},
			wantChange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := resourceApp().Diff(context.Background(), appStateBeforePreserve(t), appConfig(tt.service), nil)
			if err != nil {
				t.Fatal(err)
			}
			gotChange := diff != nil && !diff.Empty()
			if gotChange != tt.wantChange {
				t.Fatalf("plan has change = %v, want %v; diff: %v", gotChange, tt.wantChange, diff)
			}
			if gotChange {
				for k := range diff.Attributes {
					if !strings.HasPrefix(k, "service.") {
						t.Errorf("unexpected change outside service: %s", k)
					}
				}
			}
		})
	}
}

func TestReadServiceContainerCount(t *testing.T) {
	horizontal := &aptibleapi.ServiceSizingPolicy{Autoscaling: "horizontal"}
	vertical := &aptibleapi.ServiceSizingPolicy{Autoscaling: "vertical"}
	preserving := map[string]interface{}{"container_count": 2, "preserve_autoscaled_container_count": true}
	notPreserving := map[string]interface{}{"container_count": 2, "preserve_autoscaled_container_count": false}

	tests := []struct {
		name         string
		prior        map[string]interface{}
		policy       *aptibleapi.ServiceSizingPolicy
		wantCount    int
		wantPreserve bool
	}{
		{"preserving with horizontal policy keeps state's count", preserving, horizontal, 2, true},
		{"preserving with vertical policy reads live count", preserving, vertical, 5, true},
		{"preserving without policy reads live count", preserving, nil, 5, true},
		{"not preserving reads live count", notPreserving, horizontal, 5, false},
		{"import has no prior service and reads live count", nil, horizontal, 5, false},
		{"state without the flag reads live count", map[string]interface{}{"container_count": 2}, horizontal, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCount, gotPreserve := readServiceContainerCount(tt.prior, 5, tt.policy)
			if gotCount != tt.wantCount || gotPreserve != tt.wantPreserve {
				t.Fatalf("got (%d, %v), want (%d, %v)", gotCount, gotPreserve, tt.wantCount, tt.wantPreserve)
			}
		})
	}
}
