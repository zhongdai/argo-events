/*
Copyright 2020 BlackRock, Inc.

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
package argo_workflow

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"os/exec"
	"testing"

	"github.com/argoproj/argo-events/common/logging"
	apicommon "github.com/argoproj/argo-events/pkg/apis/common"
	"github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

var sensorObj = &v1alpha1.Sensor{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "fake-sensor",
		Namespace: "fake",
	},
	Spec: v1alpha1.SensorSpec{
		Triggers: []v1alpha1.Trigger{
			{
				Template: &v1alpha1.TriggerTemplate{
					Name: "fake-trigger",
					K8s:  &v1alpha1.StandardK8STrigger{},
				},
			},
		},
	},
}

var (
	un = newUnstructured("argoproj.io/v1alpha1", "Workflow", "fake", "test")
)

func newUnstructured(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
				"labels": map[string]interface{}{
					"name": name,
				},
			},
		},
	}
}

func getFakeWfTrigger(operation v1alpha1.ArgoWorkflowOperation) *ArgoWorkflowTrigger {
	runtimeScheme := runtime.NewScheme()
	client := dynamicFake.NewSimpleDynamicClient(runtimeScheme)
	artifact := apicommon.NewResource(un)
	trigger := &v1alpha1.Trigger{
		Template: &v1alpha1.TriggerTemplate{
			Name: "fake",
			ArgoWorkflow: &v1alpha1.ArgoWorkflowTrigger{
				Source: &v1alpha1.ArtifactLocation{
					Resource: &artifact,
				},
				Operation: operation,
			},
		},
	}
	return NewArgoWorkflowTrigger(fake.NewSimpleClientset(), client, sensorObj.DeepCopy(), trigger, logging.NewArgoEventsLogger())
}

func TestFetchResource(t *testing.T) {
	trigger := getFakeWfTrigger("submit")
	resource, err := trigger.FetchResource(context.TODO())
	assert.Nil(t, err)
	assert.NotNil(t, resource)
	obj, ok := resource.(*unstructured.Unstructured)
	assert.Equal(t, true, ok)
	assert.Equal(t, "test", obj.GetName())
	assert.Equal(t, "Workflow", obj.GroupVersionKind().Kind)
}

func TestApplyResourceParameters(t *testing.T) {

}

func TestExecute(t *testing.T) {
	t.Run("passes trigger args as flags to argo command", func(t *testing.T) {
		ctx := context.Background()
		var actual string
		firstArg := "--foo"
		secondArg := "--bar"
		trigger := storingCmdTrigger(&actual, firstArg, secondArg)

		_, err := namespacedClientFrom(trigger).Namespace(un.GetNamespace()).Create(ctx, un, metav1.CreateOptions{})
		assert.Nil(t, err)

		_, err = trigger.Execute(ctx, nil, un)
		assert.Nil(t, err)

		expected := fmt.Sprintf("argo -n %s resume test %s %s", un.GetNamespace(), firstArg, secondArg)
		assert.Contains(t, actual, expected)
	})
}

func storingCmdTrigger(cmdStr *string, wfArgs ...string) *ArgoWorkflowTrigger {
	trigger := getFakeWfTrigger("resume")
	f := func(cmd *exec.Cmd) error {
		*cmdStr = cmd.String()
		return nil
	}
	trigger.cmdRunner = f
	trigger.Trigger.Template.ArgoWorkflow.Args = wfArgs

	return trigger
}

func namespacedClientFrom(trigger *ArgoWorkflowTrigger) dynamic.NamespaceableResourceInterface {
	return trigger.DynamicClient.Resource(schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "workflows",
	})
}
