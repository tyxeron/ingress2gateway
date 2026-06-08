/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package airlockmicrogateway_emitter

import (
	"testing"

	emitterir "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/emitter_intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEmit_Gateway(t *testing.T) {
	e := &Emitter{notify: notifications.NoopNotify}
	nn := types.NamespacedName{Namespace: "default", Name: "test-gateway"}

	gr, errs := e.Emit(emitterir.EmitterIR{
		Gateways: map[types.NamespacedName]emitterir.GatewayContext{
			nn: {
				Gateway: gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{
						Listeners: []gatewayv1.Listener{{
							Name:     "http",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: common.PtrTo(gatewayv1.Hostname("example.com")),
						}},
					},
				},
			},
		},
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if gw, ok := gr.Gateways[nn]; !ok {
		t.Fatalf("missing gateway %s", nn)
	} else if gw.Spec.GatewayClassName != emitterName {
		t.Errorf("unexpected GatewayClassName %q", gw.Spec.GatewayClassName)
	}
}
