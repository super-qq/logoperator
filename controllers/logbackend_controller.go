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
	"os"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logoperatorv1 "qi1999.io/logoperator/api/v1"
)

// LogBackendReconciler reconciles a LogBackend object
type LogBackendReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var LBM *LogBackendManager

func init() {
	InitLogBackendManager()
}

func InitLogBackendManager() {
	LBM = &LogBackendManager{
		BackEndMap: make(map[string]*SingleLogBackend),
	}
}

type LogBackendManager struct {
	BackEndMap map[string]*SingleLogBackend
	sync.RWMutex
}

func (lm *LogBackendManager) LogBackendGet(name string) (*SingleLogBackend, bool) {
	lm.RLock()
	defer lm.RUnlock()
	obj, ok := lm.BackEndMap[name]
	return obj, ok
}
func (lm *LogBackendManager) LogBackendSet(name string, sb *SingleLogBackend) {
	lm.Lock()
	defer lm.Unlock()
	lm.BackEndMap[name] = sb
}

func (lm *LogBackendManager) LogBackendDelete(name string) {
	lm.Lock()
	defer lm.Unlock()
	delete(lm.BackEndMap, name)
}

// 定义单一写入后端的结构
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

	// 获取这个LogBackend crd ,这里是检查这个 crd资源是否存在
	instance := &logoperatorv1.LogBackend{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Errorf("[ Reconcile start missing be deleted][ns:%v][LogBackend:%v]", req.NamespacedName, req.Name)
			// 如果错误是不存在，那么可能是到调谐这里 就被删了
			return reconcile.Result{}, nil
		}
		// 其它错误打印一下
		klog.Errorf("[ Reconcile start other error][err:%v][ns:%v][LogBackend:%v]", err, req.NamespacedName, req.Name)
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil

	uniqueName := req.String()

	// 获取Annotations中存储的spec对象，如果这个对象没有，说明就是新增
	//
	// oldspec := &logoperatorv1.LogBackendSpec{}
	// specStr,
	_, exists := instance.Annotations["spec"]
	if !exists {

		klog.Infof("[LogBackend.New.Add.success][ns:%v][LogBackend:%v]", err, req.Namespace, req.Name)
		// 说明就是新增,处理新增的逻辑
		slb := &SingleLogBackend{
			WriteQ:              make(chan string, instance.Spec.BufferSize),
			BufferSize:          instance.Spec.BufferSize,
			FlushSecondInterval: instance.Spec.FlushSecondInterval,
			Name:                uniqueName,
			FilePath:            fmt.Sprintf("%s-%s.log", instance.Namespace, instance.Name),
			QuitQ:               make(chan struct{}),
		}
		LBM.LogBackendSet(uniqueName, slb)
		go slb.Start()
		return ctrl.Result{}, nil
	}

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

	for {

		select {
		case <-sb.QuitQ:
			return
		case line := <-sb.WriteQ:
			writer.WriteString(line)

		}
	}

}

func (sb *SingleLogBackend) Stop() {
	klog.Infof("[SingleLogBackend.stop][sb:%v]", sb.Name)
	close(sb.QuitQ)
}
