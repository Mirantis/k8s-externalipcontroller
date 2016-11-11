import (
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
)

func Run(config *rest.Config) err {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
}
