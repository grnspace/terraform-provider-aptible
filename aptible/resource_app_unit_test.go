package aptible

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// appStateBeforePreserve returns app state written by a provider version that
// predates preserve_autoscaled_container_count.
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
