package utils

import "k8s.io/client-go/kubernetes"

type TokenProvider interface {
	RequestToken(clientset *kubernetes.Clientset, namespace, serviceAccount string) (ServiceAccountToken, error)
}
