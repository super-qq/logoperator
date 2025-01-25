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
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	logoperatorv1 "qi1999.io/logoperator/api/v1"
)

// LogBackendReconciler reconciles a LogBackend object
type LogBackendReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type SingleLogBackend struct {
	WriteQ              chan string // 写入Q，传递给采集侧用的
	BufferSize          int         // buffer 大小
	FlushSecondInterval int         // 多久刷一次盘
	Name                string      // 名字
	FilePath            string      // 日志文件地址
	QuitQ               chan struct{}
}


//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=logbackends,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=logbackends/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=logoperator.qi1999.io,resources=logbackends/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LogBackend object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LogBackendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LogBackendReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&logoperatorv1.LogBackend{}).
		Complete(r)
}

func (sb *SingleLogBackend) Start() {

	klog.Infof("[SingleLogBackend.start][sb:%v]", sb.Name)
	file, err := os.OpenFile(sb.FilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		klog.Errorf("[SingleLogBackend.start.file.open.error][sb:%v][file:%v][err:%v]", sb.Name, sb.FilePath, err)
		return
	}
        // 关闭句柄
        defer file.close()

        writer := bufio.NewWriterSize(file, sb.BufferSize)

        flushTicker := time.NewTicker(time.Duration(sb.FlushSecondInterval) * time.Second)
        defer writer.Flush()

	go func() {
		// 开启定时flush日志到文件的任务
		for {
			select {
			case <-sb.QuitQ:
				return
			case <-flushTicker.C:
				// TODO remove this 这里可以开个测试 
				for i := 0; i < 100; i++ {
					line := fmt.Sprintf("%s-%d-%s-%s\n",
						time.Now().Format("2006-01-02 15:04:05"),
						i, sb.FilePath, "testlog")
					writer.WriteString(line)
				}

				writer.Flush()
			}
		}

	}()


}





