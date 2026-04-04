package utils

import (
	"context"
	"fmt"
	"sync"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ServiceAccountTokenContainer struct {
	Mu    *sync.RWMutex
	Token ServiceAccountToken
}

func RequestToken(clientset *kubernetes.Clientset, namespace, serviceAccount string) (ServiceAccountToken, error) {
	ctx := context.Background()
	expirationSeconds := int64(60 * 60) // 1 hour
	tokenRequest := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{"api"},
			ExpirationSeconds: &expirationSeconds,
		},
	}

	resp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, serviceAccount, tokenRequest, metav1.CreateOptions{})

	if err != nil {
		return "", fmt.Errorf("Failed to request token: %v\n", err)
	}

	return ServiceAccountToken(resp.Status.Token), nil
}

func CreateClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Failed to create inCluster config: %v\n", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to create k8s clientset: %v\n", err)
	}

	return clientset, nil
}
