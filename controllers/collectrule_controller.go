/*
Copyright 2025.

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

package controllers

import (
	"bufio"
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetCore "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logoperatorv1 "qi1999.io/logoperator/api/v1"
)

// CollectRuleReconciler reconciles a CollectRule object
type CollectRuleReconciler struct {
	CoreClientSet *clientsetCore.Clientset // 操作core 对象的client
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=collectrules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=collectrules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=collectrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CollectRule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CollectRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 获取这个CollectRule crd ,这里是检查这个 crd资源是否存在
	instance := &logoperatorv1.CollectRule{}

	klog.Infof("[Reconcile call  start][ns:%v][CollectRule:%v]", req.Namespace, req.Name)
	// 唯一的标识 ，这里用namespace+name 是为了防止同名对象出现在多个ns中
	//uniqueName := req.NamespacedName.String()
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Errorf("[ Reconcile start missing be deleted][ns:%v][CollectRule:%v]", req.Namespace, req.Name)
			// 如果错误是不存在，那么可能是到调谐这里 就被删了
			return reconcile.Result{}, nil
		}
		// 其它错误打印一下
		klog.Errorf("[ Reconcile start other error][err:%v][ns:%v][CollectRule:%v]", err, req.Namespace, req.Name)
		return reconcile.Result{}, err
	}

	// 获取配置的logbackend crd 是否存在
	lb := &logoperatorv1.LogBackend{}

	lbKey := types.NamespacedName{
		Namespace: instance.Spec.TargetNamespace,
		Name:      instance.Spec.LogBackend,
	}
	lbUniqueName := lbKey.String()
	fmt.Print(lbUniqueName)
	err = r.Client.Get(context.TODO(), lbKey, lb)
	if err != nil {

		// 这里不需要再判断是否是 未找到了，因为网络错误和未找到都相当于 lb不存在，需要再入队判断
		klog.Errorf("[ get LogBackend for CollectRule error][err:%v][ns:%v][CollectRule:%v][lb:%v]", err, req.Namespace, req.Name, lbKey)
		// return reconcile.Result{}, err
		return reconcile.Result{Requeue: true}, nil
		//return reconcile.Result{RequeueAfter: time.Second * 5}, nil
	}

	oldspec := &logoperatorv1.CollectRuleSpec{}
	specStr, exists := instance.Annotations["spec"]
	fmt.Println(oldspec, specStr)
	if !exists {

		// 处理新增的逻辑
		//klog.Infof("[CollectRule.New.Add.success][ns:%v][CollectRule:%v]", req.Namespace, req.Name)
		// 首先根据 yaml配置的labelSelector 获取对应的pod
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(instance.Spec.TargetNamespace),
			client.MatchingLabels(instance.Spec.Selector),
		}
		if err = r.List(ctx, podList, listOpts...); err != nil {
			klog.Errorf("[ Failed to list pods][ns:%v][CollectRule:%v][err:%v]", req.Namespace, req.Name, err)
			return ctrl.Result{}, err
		}
		klog.Infof("[CollectRule.New.Get.pod.result][ns:%v][CollectRule:%v][podNum:%v]", req.Namespace, req.Name, len(podList.Items))

		for _, p := range podList.Items {
			p := p
			klog.Infof("[CollectRule.New.Get.pod.result.one][ns:%v][CollectRule:%v][pod:%v]", req.Namespace, req.Name, p.Name)
			opts := &corev1.PodLogOptions{
				Follow: false, // 对应kubectl logs -f参数
			}
			logRequest := r.CoreClientSet.CoreV1().Pods(instance.Spec.TargetNamespace).GetLogs(p.Name, opts)
			readCloser, err := logRequest.Stream(context.TODO())
			if err != nil {
				klog.Errorf("[CollectRule.New.pod.GetLogs.err][ns:%v][CollectRule:%v][pod:%v][err:%v]", req.Namespace, req.Name, p.Name, err)
				continue
			}
			r := bufio.NewReader(readCloser)
			for {
				bytes, err := r.ReadBytes('\n')
				klog.Infof("[CollectRule.New.pod.GetLogs.line.print][ns:%v][CollectRule:%v][pod:%v][line:%v]", req.Namespace, req.Name, p.Name, string(bytes))
				if err != nil {
					if err != io.EOF {
						break
					}
					break
				}
			}
			readCloser.Close()

		}

	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CollectRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&logoperatorv1.CollectRule{}).
		Complete(r)
}
