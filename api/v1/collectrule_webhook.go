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

package v1

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var collectrulelog = logf.Log.WithName("collectrule-resource")

func (r *CollectRule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-logoperator-qi1999-io-v1-collectrule,mutating=true,failurePolicy=fail,sideEffects=None,groups=logoperator.qi1999.io,resources=collectrules,verbs=create;update,versions=v1,name=mcollectrule.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &CollectRule{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CollectRule) Default() {
	collectrulelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-logoperator-qi1999-io-v1-collectrule,mutating=false,failurePolicy=fail,sideEffects=None,groups=logoperator.qi1999.io,resources=collectrules,verbs=create;update,versions=v1,name=vcollectrule.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &CollectRule{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *CollectRule) ValidateCreate() error {
	collectrulelog.Info("validate create", "name", r.Name)

	/*

		apiVersion: logoperator.ning1875.io/v1
		kind: CollectRule
		metadata:
		  name: cr-01
		spec:
		  logBackend: lb-1
		  targetNamespace: default # 过滤namespace
		  selector:
		    app: gin-web-log # 过滤pod
		  collectPattern: ".*xiaoyi.*"   # 过滤日志的正则
		  logAlert:
		    url: https://oapi.dingtalk.com/robot/send?access_token=75f08bf6f2fa40d45bc987608fa3ffa860bc9d8e2cd2b6099a5cc644ba0b3c50
		    keyWordPattern: ".*400.*"
		    atMobiles:
		      - "15810947075"
	*/
	// 验证的点01  collectPattern 正则是否无法编译
	// 验证的点02  logAlert 不为空，则要求url和keyWordPattern存在，但atMobiles可以为空
	// 验证的点03  logAlert 不为空，  keyWordPattern正则是否无法编译
	// 验证的点04  logAlert 不为空，要求url 可以被rl.ParseRequestURI检查
	// 验证的点05  logAlert 不为空，atMobiles  手机号是否有问题

	// 验证的点01  collectPattern 正则是否无法编译
	if r.Spec.CollectPattern != "" {
		_, err := regexp.Compile(r.Spec.CollectPattern)
		if err != nil {
			klog.Errorf("[CollectPattern.regexp.Compile.err][CollectRule:%v][err:%v]", r, err)
			return err
		}

	}

	if r.Spec.LogAlert != nil {
		// 验证的点02  logAlert 不为空，则要求url和keyWordPattern存在，但atMobiles可以为空
		if r.Spec.LogAlert.KeyWordPattern == "" {
			err := fmt.Errorf("LogAlert.need.KeyWordPattern")
			klog.Errorf("[CollectPattern.LogAlert.need.KeyWordPattern.err][CollectRule:%v][err:%v]", r, err)
			return err
		}
		if r.Spec.LogAlert.Url == "" {
			err := fmt.Errorf("LogAlert.need.Url")
			klog.Errorf("[CollectPattern.LogAlert.need.Url.err][CollectRule:%v][err:%v]", r, err)
			return err
		}
		if strings.HasPrefix(r.Spec.LogAlert.Url, "http") {
			err := fmt.Errorf("LogAlert.start.http")

			klog.Errorf("[CollectPattern.LogAlert.need.Url.err][CollectRule:%v][err:%v]", r, err)
			return err
		}
		// 验证的点04  logAlert 不为空，要求url 可以被rl.ParseRequestURI检查
		_, err := url.ParseRequestURI(r.Spec.LogAlert.Url)
		if err != nil {
			klog.Errorf("[CollectPattern.LogAlert.Url.invalidate][CollectRule:%v][err:%v]", r, err)
			return err
		}

		//mobile verify 验证手机号
		verifyMobileFormat := func(mobileNum string) bool {
			regular := "^((13[0-9])|(14[5,7])|(15[0-3,5-9])|(17[0,3,5-8])|(18[0-9])|166|198|199|(147))\\d{8}$"

			reg := regexp.MustCompile(regular)
			return reg.MatchString(mobileNum)

		}

		// 验证的点05  logAlert 不为空，atMobiles  手机号是否有问题
		for _, m := range r.Spec.LogAlert.AtMobiles {
			m := m
			if !verifyMobileFormat(m) {
				err := fmt.Errorf("AtMobiles.invalidate")
				klog.Errorf("[CollectPattern.LogAlert.AtMobiles.invalidate.err][CollectRule:%v][err:%v]", r, err)
				return err
			}

		}
	}
	klog.Infof("[CollectPattern.LogAlert.success][CollectRule:%v]", r)
	//if instance.Spec.LogAlert != nil {
	//	scr.KeyWordPatternReg, _ = regexp.Compile(instance.Spec.LogAlert.KeyWordPattern)
	//}

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *CollectRule) ValidateUpdate(old runtime.Object) error {
	collectrulelog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *CollectRule) ValidateDelete() error {
	collectrulelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
