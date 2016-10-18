package externalip


import (
	"github.com/golang/glog"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/pkg/tools/cache"
)


func Run(closeCh chan struct{}) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	// why old clients are using listerwatcher + reflector?
	// TODO write simple Informer that will listen on services changes and assign/unassign ips


}
