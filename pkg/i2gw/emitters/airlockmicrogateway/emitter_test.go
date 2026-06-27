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

	"github.com/google/go-cmp/cmp"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw"
	emitterir "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/emitter_intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/providers/common"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	gw, ok := gr.Gateways[nn]
	if !ok {
		t.Fatalf("missing gateway %s", nn)
	}
	if gw.Spec.GatewayClassName != emitterName {
		t.Errorf("unexpected GatewayClassName %q", gw.Spec.GatewayClassName)
	}
}

func TestEmit_warnsAndDropsUnsupportedRoutes(t *testing.T) {
	testCases := []struct {
		name                     string
		ir                       emitterir.EmitterIR
		expectedGatewayResources i2gw.GatewayResources
		expectedWarnings         int
	}{
		{
			name: "HTTPRoute is kept",
			ir: emitterir.EmitterIR{
				HTTPRoutes: map[types.NamespacedName]emitterir.HTTPRouteContext{
					{Namespace: "default", Name: "http"}: {
						HTTPRoute: gatewayv1.HTTPRoute{
							ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "http"},
						},
					},
				},
			},
			expectedGatewayResources: i2gw.GatewayResources{
				HTTPRoutes: map[types.NamespacedName]gatewayv1.HTTPRoute{
					{Namespace: "default", Name: "http"}: {
						ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "http"},
					},
				},
			},
		},
		{
			name: "unsupported routes are dropped",
			ir: emitterir.EmitterIR{
				TLSRoutes: map[types.NamespacedName]emitterir.TLSRouteContext{
					{Namespace: "default", Name: "tls"}: {
						TLSRoute: gatewayv1.TLSRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "tls"}},
					},
				},
				TCPRoutes: map[types.NamespacedName]emitterir.TCPRouteContext{
					{Namespace: "default", Name: "tcp"}: {
						TCPRoute: gatewayv1alpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "tcp"}},
					},
				},
				UDPRoutes: map[types.NamespacedName]emitterir.UDPRouteContext{
					{Namespace: "default", Name: "udp"}: {
						UDPRoute: gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "udp"}},
					},
				},
				GRPCRoutes: map[types.NamespacedName]emitterir.GRPCRouteContext{
					{Namespace: "default", Name: "grpc"}: {
						GRPCRoute: gatewayv1.GRPCRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "grpc"}},
					},
				},
			},
			expectedGatewayResources: i2gw.GatewayResources{},
			expectedWarnings:         4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var warnings int
			notify := func(mt notifications.MessageType, _ string, _ ...client.Object) {
				if mt == notifications.WarningNotification {
					warnings++
				}
			}
			e := &Emitter{notify: notify}

			gr, errs := e.Emit(tc.ir)
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if warnings != tc.expectedWarnings {
				t.Errorf("Expected %d warnings, got %d", tc.expectedWarnings, warnings)
			}

			if !apiequality.Semantic.DeepEqual(gr.HTTPRoutes, tc.expectedGatewayResources.HTTPRoutes) {
				t.Errorf("HTTPRoutes mismatch (-want +got):\n%s", cmp.Diff(tc.expectedGatewayResources.HTTPRoutes, gr.HTTPRoutes))
			}

			if len(gr.TLSRoutes) != len(tc.expectedGatewayResources.TLSRoutes) {
				t.Errorf("Expected %d TLSRoutes, got %d", len(tc.expectedGatewayResources.TLSRoutes), len(gr.TLSRoutes))
			}
			if len(gr.TCPRoutes) != len(tc.expectedGatewayResources.TCPRoutes) {
				t.Errorf("Expected %d TCPRoutes, got %d", len(tc.expectedGatewayResources.TCPRoutes), len(gr.TCPRoutes))
			}
			if len(gr.UDPRoutes) != len(tc.expectedGatewayResources.UDPRoutes) {
				t.Errorf("Expected %d UDPRoutes, got %d", len(tc.expectedGatewayResources.UDPRoutes), len(gr.UDPRoutes))
			}
			if len(gr.GRPCRoutes) != len(tc.expectedGatewayResources.GRPCRoutes) {
				t.Errorf("Expected %d GRPCRoutes, got %d", len(tc.expectedGatewayResources.GRPCRoutes), len(gr.GRPCRoutes))
			}
		})
	}
}

func TestEmit_applyRegularExpressionPathMatchFeature(t *testing.T) {
	gwName := types.NamespacedName{Namespace: "default", Name: "gw"}
	routeName := types.NamespacedName{Namespace: "default", Name: "route"}
	secondRouteName := types.NamespacedName{Namespace: "default", Name: "route2"}

	paramsRef := func(name string) *gatewayv1.LocalParametersReference {
		return &gatewayv1.LocalParametersReference{
			Group: gatewayv1.Group(gatewayParametersGVK.Group),
			Kind:  gatewayv1.Kind(gatewayParametersGVK.Kind),
			Name:  name,
		}
	}

	testCases := []struct {
		name                         string
		gr                           i2gw.GatewayResources
		wantExtensions               []unstructured.Unstructured
		wantGatewaysWithGatewayInfra map[types.NamespacedName]*gatewayv1.GatewayInfrastructure
	}{
		{
			name: "no regex path match does not modify Gateway",
			gr: i2gw.GatewayResources{
				Gateways: map[types.NamespacedName]gatewayv1.Gateway{
					gwName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: gwName.Namespace, Name: gwName.Name},
					},
				},
				HTTPRoutes: map[types.NamespacedName]gatewayv1.HTTPRoute{
					routeName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: routeName.Namespace, Name: routeName.Name},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gwName.Name)}},
							},
							Rules: []gatewayv1.HTTPRouteRule{{
								Matches: []gatewayv1.HTTPRouteMatch{{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
				},
			},
		},
		{
			name: "regex path match patches Gateway and adds GatewayParameters",
			gr: i2gw.GatewayResources{
				Gateways: map[types.NamespacedName]gatewayv1.Gateway{
					gwName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: gwName.Namespace, Name: gwName.Name},
					},
				},
				HTTPRoutes: map[types.NamespacedName]gatewayv1.HTTPRoute{
					routeName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: routeName.Namespace, Name: routeName.Name},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gwName.Name)}},
							},
							Rules: []gatewayv1.HTTPRouteRule{{
								Matches: []gatewayv1.HTTPRouteMatch{{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
				},
			},
			wantExtensions: []unstructured.Unstructured{gwParametersWithRegexPathMatchEnabled("default", "gw-params")},
			wantGatewaysWithGatewayInfra: map[types.NamespacedName]*gatewayv1.GatewayInfrastructure{
				gwName: {ParametersRef: paramsRef("gw-params")},
			},
		},
		{
			name: "multiple HTTPRoutes with regex path matching on same Gateway create only one GatewayParameters",
			gr: i2gw.GatewayResources{
				Gateways: map[types.NamespacedName]gatewayv1.Gateway{
					gwName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: gwName.Namespace, Name: gwName.Name},
					},
				},
				HTTPRoutes: map[types.NamespacedName]gatewayv1.HTTPRoute{
					routeName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: routeName.Namespace, Name: routeName.Name},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gwName.Name)}},
							},
							Rules: []gatewayv1.HTTPRouteRule{{
								Matches: []gatewayv1.HTTPRouteMatch{{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
					secondRouteName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: secondRouteName.Namespace, Name: secondRouteName.Name},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gwName.Name)}},
							},
							Rules: []gatewayv1.HTTPRouteRule{{
								Matches: []gatewayv1.HTTPRouteMatch{{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
				},
			},
			wantExtensions: []unstructured.Unstructured{gwParametersWithRegexPathMatchEnabled("default", "gw-params")},
			wantGatewaysWithGatewayInfra: map[types.NamespacedName]*gatewayv1.GatewayInfrastructure{
				gwName: {ParametersRef: paramsRef("gw-params")},
			},
		},
		{
			name: "existing infrastructure config is preserved when patching ParametersRef",
			gr: i2gw.GatewayResources{
				Gateways: map[types.NamespacedName]gatewayv1.Gateway{
					gwName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: gwName.Namespace, Name: gwName.Name},
						Spec: gatewayv1.GatewaySpec{Infrastructure: &gatewayv1.GatewayInfrastructure{
							Labels: map[gatewayv1.LabelKey]gatewayv1.LabelValue{"k": "v"},
						}},
					},
				},
				HTTPRoutes: map[types.NamespacedName]gatewayv1.HTTPRoute{
					routeName: {
						ObjectMeta: metav1.ObjectMeta{Namespace: routeName.Namespace, Name: routeName.Name},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gwName.Name)}},
							},
							Rules: []gatewayv1.HTTPRouteRule{{
								Matches: []gatewayv1.HTTPRouteMatch{{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
				},
			},
			wantExtensions: []unstructured.Unstructured{gwParametersWithRegexPathMatchEnabled("default", "gw-params")},
			wantGatewaysWithGatewayInfra: map[types.NamespacedName]*gatewayv1.GatewayInfrastructure{
				gwName: {
					Labels:        map[gatewayv1.LabelKey]gatewayv1.LabelValue{"k": "v"},
					ParametersRef: paramsRef("gw-params"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Emitter{notify: notifications.NoopNotify}
			e.applyRegularExpressionPathMatchFeature(&tc.gr)

			if !apiequality.Semantic.DeepEqual(tc.gr.GatewayExtensions, tc.wantExtensions) {
				t.Errorf("GatewayExtensions mismatch (-want +got):\n%s", cmp.Diff(tc.wantExtensions, tc.gr.GatewayExtensions))
			}

			for key, g := range tc.gr.Gateways {
				wantInfra := tc.wantGatewaysWithGatewayInfra[key]
				if !apiequality.Semantic.DeepEqual(g.Spec.Infrastructure, wantInfra) {
					t.Errorf("Gateway %s Infrastructure mismatch (-want +got):\n%s", key, cmp.Diff(wantInfra, g.Spec.Infrastructure))
				}
			}
		})
	}
}
