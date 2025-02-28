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
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logoperatorv1 "qi1999.io/logoperator/api/v1"
)

// LogBackendReconciler reconciles a LogBackend object
type LogBackendReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LogbackendDir string
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

// 封装一个初始化单一写入后端的方法
func NewSingleLogBackend(buffsize int, flushSecondInterval int, name string, filePath string) *SingleLogBackend {
	return &SingleLogBackend{
		WriteQ:              make(chan string, buffsize),
		BufferSize:          buffsize,
		FlushSecondInterval: flushSecondInterval,
		Name:                name,
		FilePath:            filePath,
		QuitQ:               make(chan struct{}),
	}

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
	klog.Infof("[Reconcile call start][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
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

	uniqueName := req.String()

	// 处理删除的逻辑，定义1个finalizers
	myFinalizerName := "storage.finalizers.qi1999.io"

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// DeletionTimestamp为0 说明不是删除的，需要判断对象原有的Finalizers有没有上面定义的
		if !containsString(instance.ObjectMeta.Finalizers, myFinalizerName) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// 有DeletionTimestamp说明要删除,比如，离职审批
		if containsString(instance.ObjectMeta.Finalizers, myFinalizerName) {
			// 新增一个deleteExternalDependency 来模拟删除这个lb时很慢
			if err := r.deleteExternalDependency(instance); err != nil {
				klog.Errorf("[deleteExternalDependency.to.lb.err][err:%v][ns:%v][LogBackend:%v]", err, req.Namespace, req.Name)
				return reconcile.Result{}, err
			}

			oldSlb, ok := LBM.LogBackendGet(uniqueName)
			if !ok {
				klog.Errorf("[LogBackend.Delete.notfound.Inlocal.error][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
				return ctrl.Result{}, nil
			}
			// 先停止旧的
			oldSlb.Stop()
			// 在map中删除
			LBM.LogBackendDelete(uniqueName)
			klog.Infof("LogBackend.Delete.old.stop[ns:%v][LogBackend:%v][meta:%v]", req.Namespace, req.Name, oldSlb)
			// 增加一个sleep ，模拟很慢，并且把原来的deleteExternalDependency注释掉
			time.Sleep(10 * time.Second)

			// removeString 把myFinalizerName从Finalizers列表中移除，因为我们已经执行完了，然后更新对象
			instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				klog.Errorf("[deleteExternalDependency.exec.success.update.lb.error][err:%v][ns:%v][LogBackend:%v]", err, req.Namespace, req.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// 获取Annotations中存储的spec对象，如果这个对象没有，说明就是新增
	//
	// oldspec := &logoperatorv1.LogBackendSpec{}
	// specStr,
	oldspec := &logoperatorv1.LogBackendSpec{}
	specStr, specExists := instance.Annotations["spec"]
	_, slbExists := LBM.LogBackendGet(uniqueName)
	if !specExists || !slbExists {

		klog.Infof("[LogBackend.New.Add.success][ns:%v][LogBackend:%v]", err, req.Namespace, req.Name)
		// 说明就是新增,处理新增的逻辑
		slb := NewSingleLogBackend(
			instance.Spec.BufferSize,
			instance.Spec.FlushSecondInterval,
			uniqueName,
			fmt.Sprintf("%s/%s-%s.log", r.LogbackendDir, instance.Namespace, instance.Name),
		)

		LBM.LogBackendSet(uniqueName, slb)
		go slb.Start()

		// 3. 关联 Annotations
		data, err := json.Marshal(instance.Spec)
		if err != nil {
			klog.Error(err, "LogBackend.Spec.json.Marshal.err")
			return reconcile.Result{}, nil
		}
		klog.Info("create.LogBackend.Spec.data.string", "spce", string(data))
		if instance.Annotations != nil {
			instance.Annotations["spec"] = string(data)
		} else {
			instance.Annotations = map[string]string{"spec": string(data)}
		}
		if err := r.Client.Update(context.TODO(), instance); err != nil {
			klog.Errorf("LogBackend.update.err[ns:%v][LogBackend:%v]", req.Namespace, req.Name)
			return reconcile.Result{}, err
		}
		klog.Infof("[LogBackend.update.success][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
		instance.Status.SyncPhase = logoperatorv1.StatusLogBackendRunning
		instance.Status.LastDeployTime = &metav1.Time{Time: time.Now().UTC()}
		err = r.Status().Update(ctx, instance)
		if err != nil {
			klog.Errorf("[logbackend.updateStatus.err][ns:%v][LogBackend:%v][err:%v]", req.Namespace, req.Name, err)
			return reconcile.Result{}, err
		}
		klog.Infof("[LogBackend.New.Add.success][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
		return ctrl.Result{}, nil
	}

	// 走到这里说明spec str存在，需要对比一下是否变更了
	if err := json.Unmarshal([]byte(specStr), &oldspec); err != nil {
		klog.Errorf("[LogBackend.oldspec.json.Unmarshal.err][ns:%v][LogBackend:%v][err:%v]", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}
	// 没变化
	if reflect.DeepEqual(&instance.Spec, oldspec) {
		klog.Errorf("[LogBackend.reflect.DeepEqual][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
		return ctrl.Result{}, nil
	}

	// 变化了
	specOldData := instance.Annotations["spec"]
	specNewData, _ := json.Marshal(instance.Spec)
	klog.V(2).Infof("LogBackend.instance.Spec.data.diff[ns:%v][LogBackend:%v[specOldData:%v][specNewData:%v]", req.Namespace, req.Name, specOldData, string(specNewData))

	oldSlb, ok := LBM.LogBackendGet(uniqueName)
	if !ok {
		klog.Errorf("[LogBackend.notfoundInlocal.DeepEqual][ns:%v][LogBackend:%v]", req.Namespace, req.Name)
		return ctrl.Result{}, nil
	}
	// 先停止旧的
	oldSlb.Stop()
	// 在map中删除
	LBM.LogBackendDelete(uniqueName)
	klog.Infof("LogBackend.Update.old.stop[ns:%v][LogBackend:%v][meta:%v]", req.Namespace, req.Name, oldSlb)

	// 开启新的
	newSlb := NewSingleLogBackend(
		instance.Spec.BufferSize,
		instance.Spec.FlushSecondInterval,
		uniqueName,
		fmt.Sprintf("%s/%s-%s.log", r.LogbackendDir, instance.Namespace, instance.Name),
	)
	LBM.LogBackendSet(uniqueName, newSlb)
	go newSlb.Start()
	klog.Infof("LogBackend.Update.new.start[ns:%v][LogBackend:%v][meta:%v]", req.Namespace, req.Name, newSlb)

	if instance.Annotations != nil {
		instance.Annotations["spec"] = string(specNewData)
	} else {
		instance.Annotations = map[string]string{"spec": string(specNewData)}
	}
	if err := r.Client.Update(context.TODO(), instance); err != nil {
		klog.Errorf("LogBackend.update.err[ns:%v][LogBackend:%v]", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}
	klog.Infof("[LogBackend.Update.updateSpec.success][ns:%v][LogBackend:%v]", req.Namespace, req.Name)

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
	defer file.Close()

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
				// for i := 0; i < 100; i++ {
				//	line := fmt.Sprintf("%s-%d-%s-%s\n",
				//		time.Now().Format("2006-01-02 15:04:05"),
				//		i, sb.FilePath, "testlog")
				//	writer.WriteString(line)
				//}

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

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func (r *LogBackendReconciler) deleteExternalDependency(instance *logoperatorv1.LogBackend) error {

	klog.Infof("start deleting the external dependencies for lb:%v", instance.Name)
	time.Sleep(10 * time.Second)
	klog.Infof("end deleting the external dependencies for lb:%v", instance.Name)
	return nil
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
