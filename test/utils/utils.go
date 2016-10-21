package utils

import (
	"flag"
	"fmt"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/clientcmd"

	"github.com/golang/glog"
	. "github.com/onsi/gomega"
)

var KUBECONFIG string

func init() {
	flag.StringVar(&KUBECONFIG, "kubeconfig", "/etc/kubeconfig", "kubeconfig to use with kubernetes client")
}

func KubeClient() (*kubernetes.Clientset, error) {
	glog.Infof("Using config %v\n", KUBECONFIG)
	config, err := clientcmd.BuildConfigFromFlags("", KUBECONFIG)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func WaitForReady(clientset *kubernetes.Clientset, pod *v1.Pod) {
	Eventually(func() error {
		podUpdated, err := clientset.Core().Pods(pod.Namespace).Get(pod.Name)
		if err != nil {
			return err
		}
		if podUpdated.Status.Phase != v1.PodRunning {
			return fmt.Errorf("pod %v is not running phase: %v", podUpdated.Name, podUpdated.Status.Phase)
		}
		return nil
	}, 120*time.Second, 5*time.Second).Should(BeNil())
}
