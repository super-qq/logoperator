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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	clientsetCore "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logoperatorv1 "qi1999.io/logoperator/api/v1"
)

const DingAlertTitle = "logoperator钉钉告警"

// 定义规则的本地管理器
var CLM *CollectRuleManager

// 初始化方法
func init() {
	InitCollectRuleManager()
}

// 初始化方法
func InitCollectRuleManager() {
	CLM = &CollectRuleManager{
		CLMap: make(map[string]*SingleCollectRule),
	}
}

// 基于map的管理器
type CollectRuleManager struct {
	CLMap map[string]*SingleCollectRule
	sync.RWMutex
}

type SingleCollectRule struct {
	Name          string                        // CollectRule的名字
	Spec          logoperatorv1.CollectRuleSpec // CollectRule的Spec配置
	Slb           *SingleLogBackend             // 对应写入后端
	Ctx           context.Context               // context
	QuitQ         chan struct{}                 // 退出的chan
	CoreClientSet *clientsetCore.Clientset      // 操作core 对象的client

	CollectPatternReg *regexp.Regexp `json:"-" yaml:"-" ` //给内部用的采集正则表达式
	KeyWordPatternReg *regexp.Regexp `json:"-" yaml:"-" ` //给内部用的告警正则表达式

}

// map 的get set delete 方法
func (lm *CollectRuleManager) CollectRuleGet(name string) (*SingleCollectRule, bool) {
	lm.RLock()
	defer lm.RUnlock()
	obj, ok := lm.CLMap[name]
	return obj, ok
}
func (lm *CollectRuleManager) CollectRuleSet(name string, sb *SingleCollectRule) {
	lm.Lock()
	defer lm.Unlock()
	lm.CLMap[name] = sb
}

func (lm *CollectRuleManager) CollectRuleDelete(name string) {
	lm.Lock()
	defer lm.Unlock()
	delete(lm.CLMap, name)
}

// 发送钉钉消息的结构体
type dingMsgNew struct {
	Msgtype string  `json:"msgtype"`
	Text    content `json:"text"`
	At      at      `json:"at"`
}
type content struct {
	Content string `json:"content"`
}
type at struct {
	AtMobiles []string `json:"atMobiles"`
}

func DingDingMsgDirectSend(app, msg string, apiUrl string, atMobiles []string) {

	dm := dingMsgNew{Msgtype: "text"}
	dm.Text.Content = fmt.Sprintf("[%s]\n[app=%s]\n"+
		"[日志详情:%s ]", DingAlertTitle, app, msg)
	dm.At.AtMobiles = atMobiles
	bs, err := json.Marshal(dm)
	if err != nil {
		klog.Errorf(
			"[msg:dingding.msg.json.Marshal.error][dm:%v][err:%v]", dm, err)
		return
	}
	res, err := http.Post(apiUrl, "application/json", bytes.NewBuffer(bs))
	if err != nil {
		klog.Errorf("[send.dingding.error][err:%v][msg:v]", err, msg)
		return
	}
	if res != nil && res.StatusCode != 200 {
		klog.Errorf("[send.dingding.status_code.error][code:%v][msg:v]", res.StatusCode, msg)
		return
	}
	klog.Infof("[send.dingding.success][code:%v][msg:v]", res.StatusCode, msg)

}

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
		// return reconcile.Result{Requeue: true}, nil
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		//return reconcile.Result{RequeueAfter: time.Second * 5}, nil
	}

	slb, exists := LBM.LogBackendGet(lbUniqueName)

	if !exists {
		klog.Errorf("[slb.not.exist.in.localMap.mayBe.lb.controller.delay][ns:%v][CollectRule:%v][lb:%v]", req.Namespace, req.Name, lbKey)
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	scr := &SingleCollectRule{
		Name:          instance.Name,
		Spec:          instance.Spec,
		Slb:           slb,
		CoreClientSet: r.CoreClientSet,
		Ctx:           ctx,
		QuitQ:         make(chan struct{}),
	}

	if instance.Spec.CollectPattern != "" {
		scr.CollectPatternReg, _ = regexp.Compile(instance.Spec.CollectPattern)
	}

	if instance.Spec.LogAlert != nil {
		scr.KeyWordPatternReg, _ = regexp.Compile(instance.Spec.LogAlert.KeyWordPattern)
	}

	// 获取Annotations中存储的spec对象，如果这个对象没有，说明就是新增
	//
	oldspec := &logoperatorv1.CollectRuleSpec{}
	specStr, specExists := instance.Annotations["spec"]
	_, localCrExists := CLM.CollectRuleGet(req.Name)

	fmt.Println(oldspec, specStr)
	// spec 或者本地map中不存在都认为是新增，防止控制器重启之前running 的实例没人管
	if !specExists || !localCrExists {
		go scr.Start()
		CLM.CollectRuleSet(instance.Name, scr)
		klog.Infof("[CollectRule.New.Add.success][ns:%v][CollectRule:%v	]", req.Namespace, req.Name)
	}

	return reconcile.Result{}, nil
}

func (sr *SingleCollectRule) Stop() {
	close(sr.QuitQ)
}

func (sr *SingleCollectRule) Start() {

	// 处理新增的逻辑
	//klog.Infof("[CollectRule.New.Add.success][ns:%v][CollectRule:%v]", req.Namespace, req.Name)
	// 首先根据 yaml配置的labelSelector 获取对应的pod
	ns := sr.Spec.TargetNamespace
	watchHandler, err := sr.CoreClientSet.CoreV1().Pods(sr.Spec.TargetNamespace).Watch(sr.Ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(sr.Spec.Selector),
	})

	if err != nil {
		klog.Errorf("[ Failed to watch pods][ns:%v][CollectRule:%v][err:%v]", ns, sr.Name, err)
		return
	}

	for {
		select {
		case <-sr.QuitQ:
			klog.Infof("[ CollectRule.exit.readPodLog.exit][ns:%v][CollectRule:%v][err:%v]", ns, sr.Name, err)
			return
		default:
			for event := range watchHandler.ResultChan() {
				pod, ok := event.Object.(*corev1.Pod)
				if !ok {
					continue
				}
				// 判断event的类型
				switch event.Type {
				case watch.Added:
					go func() {
						// 获取logRequest.Stream
						opts := &corev1.PodLogOptions{
							Follow: true, // 对应kubectl logs -f参数
						}
						logRequest := sr.CoreClientSet.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)
						readCloser, err := logRequest.Stream(context.TODO())
						if err != nil {
							klog.Errorf("[pod.logRequest.Stream.err][ns:%v][CollectRule:%v][pod:%v][err:%v]", pod.Namespace, sr.Name, pod.Name, err)
							return
						}
						defer readCloser.Close()
						r := bufio.NewReader(readCloser)

						for {
							bytes, err := r.ReadBytes('\n')
							if err != nil {
								if err != io.EOF {
									klog.Errorf("[CollectRule.New.pod.ReadLogs.err][ns:%v][CollectRule:%v][pod:%v][err:%v]", pod.Namespace, sr.Name, pod.Name, err)
									break
								}

								break
							}
							oneline := strings.TrimRight(string(bytes), "\n")

							// 将这个告警正则匹配放到主正则的上面，避免被主正则过滤
							if sr.KeyWordPatternReg != nil {
								//	处理日志采集正则
								v := sr.KeyWordPatternReg.FindStringSubmatch(oneline)

								/*
									## 处理日志主正则
									- patternReg.FindStringSubmatch(line) 的结果v
									- len=0 说明 正则没匹配中，应该丢弃这行
									- len=1 说明 正则匹配中了，但是小括号分组没匹配到
									- len>1 说明 正则匹配中了，小括号分组也匹配到
								*/
								if len(v) > 0 {
									//	正则匹配中，应该处理发送这一行
									// 这里可以打一行匹配命中的日志
									klog.Infof("[CollectRule.New.pod.GetLogs.line.keyWordRegex.need.send][ns:%v][CollectRule:%v][pod:%v][reg:%v][line:%v]", pod.Namespace, sr.Name, pod.Name, sr.KeyWordPatternReg, oneline)

									//构造app
									app := fmt.Sprintf("[podNs:%v pod:%v]", pod.Namespace,
										pod.Name)
									// 异步发送
									go sr.sendDingDing(app, oneline)

								}

							}1

							//	 判断采集正则是否存在
							if sr.CollectPatternReg != nil {
								//	处理日志采集正则
								v := sr.CollectPatternReg.FindStringSubmatch(oneline)

								/*
									## 处理日志主正则
									- patternReg.FindStringSubmatch(line) 的结果v
									- len=0 说明 正则没匹配中，应该丢弃这行
									- len=1 说明 正则匹配中了，但是小括号分组没匹配到
									- len>1 说明 正则匹配中了，小括号分组也匹配到
								*/
								if len(v) == 0 {
									//	 正则没匹配中，应该丢弃这行
									// 这里可以打一行丢弃的日志

									klog.Infof("[CollectRule.New.pod.GetLogs.line.regex.not.match][ns:%v][CollectRule:%v][pod:%v][reg:%v][line:%v]", pod.Namespace, sr.Name, pod.Name, sr.CollectPatternReg, oneline)

									continue
								}

							}

							newLine := fmt.Sprintf("[collectRule:%v podNs:%v pod:%v][line:%v]",
								sr.Name,
								pod.Namespace,
								pod.Name,
								oneline,
							)
							klog.Infof("[CollectRule.New.pod.GetLogs.line.print][ns:%v][CollectRule:%v][pod:%v][line:%v]", pod.Namespace, sr.Name, pod.Name, newLine)
							sr.Slb.WriteQ <- newLine

						}

					}()
				case watch.Deleted:
				}
			}
		}
	}
}

//给这个结构体绑定一个方法发送钉钉告警

func (sr *SingleCollectRule) sendDingDing(app, msg string) {
	DingDingMsgDirectSend(app, msg, sr.Spec.LogAlert.Url, sr.Spec.LogAlert.AtMobiles)

}

// SetupWithManager sets up the controller with the Manager.
func (r *CollectRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&logoperatorv1.CollectRule{}).
		Complete(r)
}
