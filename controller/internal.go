package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services"
	"github.com/rancher/eks-operator/utils"
	wranglerv1 "github.com/rancher/wrangler/v2/pkg/generated/controllers/core/v1"
)

func newAWSConfigV2(ctx context.Context, secretsCache wranglerv1.SecretCache, spec eksv1.EKSClusterConfigSpec) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return cfg, fmt.Errorf("error loading default AWS config: %v", err)
	}

	if region := spec.Region; region != "" {
		cfg.Region = region
	}

	ns, id := utils.Parse(spec.AmazonCredentialSecret)
	if amazonCredentialSecret := spec.AmazonCredentialSecret; amazonCredentialSecret != "" {
		secret, err := secretsCache.Get(ns, id)
		if err != nil {
			return cfg, fmt.Errorf("error getting secret %s/%s: %w", ns, id, err)
		}

		accessKeyBytes := secret.Data["amazonec2credentialConfig-accessKey"]
		secretKeyBytes := secret.Data["amazonec2credentialConfig-secretKey"]
		if accessKeyBytes == nil || secretKeyBytes == nil {
			return cfg, fmt.Errorf("invalid aws cloud credential")
		}

		accessKey := string(accessKeyBytes)
		secretKey := string(secretKeyBytes)

		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	}

	return cfg, nil
}

func newAWSv2Services(ctx context.Context, secretsCache wranglerv1.SecretCache, spec eksv1.EKSClusterConfigSpec) (*awsServices, error) {
	cfg, err := newAWSConfigV2(ctx, secretsCache, spec)
	if err != nil {
		return nil, err
	}

	return &awsServices{
		eks:            services.NewEKSService(cfg),
		cloudformation: services.NewCloudFormationService(cfg),
		iam:            services.NewIAMService(cfg),
		ec2:            services.NewEC2Service(cfg),
	}, nil
}

func deleteStack(ctx context.Context, svc services.CloudFormationServiceInterface, newStyleName, oldStyleName string) error {
	name := newStyleName
	_, err := svc.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if doesNotExist(err) {
		name = oldStyleName
	}

	_, err = svc.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(name),
	})
	if err != nil && !doesNotExist(err) {
		return fmt.Errorf("error deleting stack: %v", err)
	}

	return nil
}
