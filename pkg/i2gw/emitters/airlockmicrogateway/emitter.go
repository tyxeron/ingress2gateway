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
	"fmt"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw"
	emitterir "github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/emitter_intermediate"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/emitters/utils"
	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	emitterName = "airlock-microgateway"

	unsupportedRouteWarningFmtStr = "dropping %s because it is unsupported by airlock-microgateway"
	gatewayParametersNameSuffix   = "-params"
)

var gatewayParametersGVK = schema.GroupVersionKind{Group: "microgateway.airlock.com", Version: "v1alpha1", Kind: "GatewayParameters"}

func init() {
	i2gw.EmitterConstructorByName[emitterName] = NewEmitter
}

type Emitter struct {
	notify notifications.NotifyFunc
}

func NewEmitter(conf *i2gw.EmitterConf) i2gw.Emitter {
	return &Emitter{
		notify: conf.Report.Notifier(emitterName),
	}
}

func (e *Emitter) Emit(ir emitterir.EmitterIR) (gr i2gw.GatewayResources, errs field.ErrorList) {
	utils.AddHTTPRouteRuleNames(ir)
	for ns, gw := range ir.Gateways {
		gw.Spec.GatewayClassName = emitterName
		ir.Gateways[ns] = gw
	}
	e.warnAndDropUnsupportedRoutes(&ir)
	gr, errs = utils.ToGatewayResources(ir)
	if len(errs) != 0 {
		return
	}
	e.applyRegularExpressionPathMatchFeature(&gr)
	utils.LogUnparsedErrors(ir, e.notify)
	return gr, nil
}

func (e *Emitter) warnAndDropUnsupportedRoutes(ir *emitterir.EmitterIR) {
	for key, r := range ir.TLSRoutes {
		e.notify(notifications.WarningNotification, fmt.Sprintf(unsupportedRouteWarningFmtStr, "TLSRoute"), &r.TLSRoute)
		delete(ir.TLSRoutes, key)
	}
	for key, r := range ir.TCPRoutes {
		e.notify(notifications.WarningNotification, fmt.Sprintf(unsupportedRouteWarningFmtStr, "TCPRoute"), &r.TCPRoute)
		delete(ir.TCPRoutes, key)
	}
	for key, r := range ir.UDPRoutes {
		e.notify(notifications.WarningNotification, fmt.Sprintf(unsupportedRouteWarningFmtStr, "UDPRoute"), &r.UDPRoute)
		delete(ir.UDPRoutes, key)
	}
	for key, r := range ir.GRPCRoutes {
		e.notify(notifications.WarningNotification, fmt.Sprintf(unsupportedRouteWarningFmtStr, "GRPCRoute"), &r.GRPCRoute)
		delete(ir.GRPCRoutes, key)
	}
}

func (e *Emitter) applyRegularExpressionPathMatchFeature(gr *i2gw.GatewayResources) {
	gwsWithRegexFeatureFlag := map[types.NamespacedName]struct{}{}

	for _, route := range gr.HTTPRoutes {
		if !httpRouteUsesRegexPathMatch(&route) {
			continue
		}
		for _, pRef := range route.Spec.ParentRefs {
			namespacedName := types.NamespacedName{Namespace: ptr.Deref((*string)(pRef.Namespace), route.Namespace), Name: string(pRef.Name)}
			gwsWithRegexFeatureFlag[namespacedName] = struct{}{}
		}
	}

	for key := range gwsWithRegexFeatureFlag {
		gw := gr.Gateways[key]
		gwParamsName := gw.Name + gatewayParametersNameSuffix

		gr.GatewayExtensions = append(gr.GatewayExtensions, gwParametersWithRegexPathMatchEnabled(gw.Namespace, gwParamsName))

		if gw.Spec.Infrastructure == nil {
			gw.Spec.Infrastructure = &gatewayv1.GatewayInfrastructure{}
		}
		gw.Spec.Infrastructure.ParametersRef = &gatewayv1.LocalParametersReference{
			Group: gatewayv1.Group(gatewayParametersGVK.Group),
			Kind:  gatewayv1.Kind(gatewayParametersGVK.Kind),
			Name:  gwParamsName,
		}
		gr.Gateways[key] = gw
	}
}

func httpRouteUsesRegexPathMatch(route *gatewayv1.HTTPRoute) bool {
	for _, rule := range route.Spec.Rules {
		for _, m := range rule.Matches {
			if m.Path != nil && m.Path.Type != nil && *m.Path.Type == gatewayv1.PathMatchRegularExpression {
				return true
			}
		}
	}
	return false
}

func gwParametersWithRegexPathMatchEnabled(namespace, name string) unstructured.Unstructured {
	gwParams := unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"features": map[string]any{
				"httpRouteRegexPathMatchEnabled": true,
			},
		},
	}}
	gwParams.SetGroupVersionKind(gatewayParametersGVK)
	gwParams.SetNamespace(namespace)
	gwParams.SetName(name)
	return gwParams
}
