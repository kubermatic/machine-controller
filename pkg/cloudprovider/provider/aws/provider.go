/*
Copyright 2019 The Machine Controller Authors.

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

package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	gocache "github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	awstypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Interval and timeout for polling.
	pollInterval = 2 * time.Second
	pollTimeout  = 5 * time.Minute
)

var (
	metricInstancesForMachines = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "machine_controller_aws_instances_for_machine",
		Help: "The number of instances at aws for a given machine"}, []string{"machine"})
)

func init() {
	metrics.Registry.MustRegister(metricInstancesForMachines)
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a aws provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

const (
	nameTag       = "Name"
	machineUIDTag = "Machine-UID"

	maxRetries = 100
)

var (
	volumeTypes = map[ec2types.VolumeType]interface{}{
		ec2types.VolumeTypeStandard: nil,
		ec2types.VolumeTypeIo1:      nil,
		ec2types.VolumeTypeGp2:      nil,
		ec2types.VolumeTypeGp3:      nil,
		ec2types.VolumeTypeSc1:      nil,
		ec2types.VolumeTypeSt1:      nil,
	}

	amiFilters = map[providerconfigtypes.OperatingSystem]map[awstypes.CPUArchitecture]amiFilter{
		// Source: https://wiki.centos.org/Cloud/AWS
		providerconfigtypes.OperatingSystemCentOS: {
			awstypes.CPUArchitectureX86_64: {
				description: "CentOS Linux 7* x86_64*",
				// The AWS marketplace ID from CentOS Community Platform Engineering (CPE)
				owner: "125523088429",
			},
			awstypes.CPUArchitectureARM64: {
				description: "CentOS Linux 7* aarch64*",
				// The AWS marketplace ID from CentOS Community Platform Engineering (CPE)
				owner: "125523088429",
			},
		},
		providerconfigtypes.OperatingSystemRockyLinux: {
			awstypes.CPUArchitectureX86_64: {
				description: "Rocky-8-ec2-8*.x86_64",
				// The AWS marketplace ID from Rocky Linux Community Platform Engineering (CPE)
				owner: "792107900819",
			},
			awstypes.CPUArchitectureARM64: {
				description: "Rocky-8-ec2-8*.aarch64",
				// The AWS marketplace ID from Rocky Linux Community Platform Engineering (CPE)
				owner: "792107900819",
			},
		},
		providerconfigtypes.OperatingSystemAmazonLinux2: {
			awstypes.CPUArchitectureX86_64: {
				description: "Amazon Linux 2 AMI * x86_64 HVM gp2",
				// The AWS marketplace ID from Amazon
				owner: "137112412989",
			},
			awstypes.CPUArchitectureARM64: {
				description: "Amazon Linux 2 LTS Arm64 AMI * arm64 HVM gp2",
				// The AWS marketplace ID from Amazon
				owner: "137112412989",
			},
		},
		providerconfigtypes.OperatingSystemUbuntu: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Canonical, Ubuntu, 22.04 LTS, amd64 jammy image build on ????-??-??",
				// The AWS marketplace ID from Canonical
				owner: "099720109477",
			},
			awstypes.CPUArchitectureARM64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Canonical, Ubuntu, 22.04 LTS, arm64 jammy image build on ????-??-??",
				// The AWS marketplace ID from Canonical
				owner: "099720109477",
			},
		},
		providerconfigtypes.OperatingSystemRHEL: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Provided by Red Hat, Inc.",
				// The AWS marketplace ID from RedHat
				owner: "309956199498",
			},
			awstypes.CPUArchitectureARM64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Provided by Red Hat, Inc.",
				// The AWS marketplace ID from RedHat
				owner: "309956199498",
			},
		},
		providerconfigtypes.OperatingSystemFlatcar: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Flatcar Container Linux stable *",
				// The AWS marketplace ID from AWS
				owner: "075585003325",
			},
			// 2021-10-14 - Flatcar stable does not support ARM yet (only alpha channels supports it)
		},
	}

	// cacheLock protects concurrent cache misses against a single key. This usually happens when multiple machines get created simultaneously
	// We lock so the first access updates/writes the data to the cache and afterwards everyone reads the cached data.
	cacheLock = &sync.Mutex{}
	cache     = gocache.New(5*time.Minute, 5*time.Minute)
)

type Config struct {
	AccessKeyID     string
	SecretAccessKey string

	Region             string
	AvailabilityZone   string
	VpcID              string
	SubnetID           string
	SecurityGroupIDs   []string
	InstanceProfile    string
	InstanceType       ec2types.InstanceType
	AMI                string
	DiskSize           int32
	DiskType           ec2types.VolumeType
	DiskIops           *int32
	EBSVolumeEncrypted bool
	Tags               map[string]string
	AssignPublicIP     *bool

	IsSpotInstance           *bool
	SpotMaxPrice             *string
	SpotPersistentRequest    *bool
	SpotInterruptionBehavior *string

	AssumeRoleARN        string
	AssumeRoleExternalID string
}

type amiFilter struct {
	description string
	owner       string
	productCode string
}

func getDefaultAMIID(ctx context.Context, client *ec2.Client, os providerconfigtypes.OperatingSystem, region string, cpuArchitecture awstypes.CPUArchitecture) (string, error) {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	osFilter, osSupported := amiFilters[os]
	if !osSupported {
		return "", fmt.Errorf("operating system %q not supported", os)
	}

	filter, archSupported := osFilter[cpuArchitecture]
	if !archSupported {
		return "", fmt.Errorf("CPU architecture '%s' not supported for operating system '%s'", cpuArchitecture, os)
	}

	cacheKey := fmt.Sprintf("ami-id-%s-%s-%s", region, os, cpuArchitecture)
	amiID, found := cache.Get(cacheKey)
	if found {
		klog.V(3).Info("found AMI-ID in cache!")
		return amiID.(string), nil
	}

	describeImagesInput := &ec2.DescribeImagesInput{
		Owners: []string{filter.owner},
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("description"),
				Values: []string{filter.description},
			},
			{
				Name:   aws.String("virtualization-type"),
				Values: []string{"hvm"},
			},
			{
				Name:   aws.String("root-device-type"),
				Values: []string{"ebs"},
			},
			{
				Name:   aws.String("architecture"),
				Values: []string{string(cpuArchitecture)},
			},
		},
	}

	if filter.productCode != "" {
		describeImagesInput.Filters = append(describeImagesInput.Filters, ec2types.Filter{
			Name:   aws.String("product-code"),
			Values: []string{filter.productCode},
		})
	}

	imagesOut, err := client.DescribeImages(ctx, describeImagesInput)
	if err != nil {
		return "", err
	}

	if len(imagesOut.Images) == 0 {
		return "", fmt.Errorf("could not find Image for '%s' with arch '%s'", os, cpuArchitecture)
	}

	if os == providerconfigtypes.OperatingSystemRHEL {
		imagesOut.Images, err = filterSupportedRHELImages(imagesOut.Images)
		if err != nil {
			return "", err
		}
	}

	image := imagesOut.Images[0]
	for _, v := range imagesOut.Images {
		itime, _ := time.Parse(time.RFC3339, *image.CreationDate)
		vtime, _ := time.Parse(time.RFC3339, *v.CreationDate)
		if vtime.After(itime) {
			image = v
		}
	}

	cache.SetDefault(cacheKey, *image.ImageId)
	return *image.ImageId, nil
}

func getCPUArchitecture(ctx context.Context, client *ec2.Client, instanceType ec2types.InstanceType) (awstypes.CPUArchitecture, error) {
	// read the instance type to know which cpu architecture is needed in the AMI
	instanceTypes, err := client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{instanceType},
	})

	if err != nil {
		return "", err
	}

	if len(instanceTypes.InstanceTypes) != 1 {
		return "", fmt.Errorf("unexpected length of instance type list: %d", len(instanceTypes.InstanceTypes))
	}

	if instanceTypes.InstanceTypes[0].ProcessorInfo != nil &&
		len(instanceTypes.InstanceTypes[0].ProcessorInfo.SupportedArchitectures) > 0 {
		for _, v := range instanceTypes.InstanceTypes[0].ProcessorInfo.SupportedArchitectures {
			// machine-controller currently supports x86_64 and ARM64, so only CPU architectures
			// that are supported will be returned if found in the AWS API response
			if arch := awstypes.CPUArchitecture(v); arch == awstypes.CPUArchitectureX86_64 || arch == awstypes.CPUArchitectureARM64 {
				return arch, nil
			}
		}
	}

	return "", errors.New("returned instance type data did not include supported architectures")
}

func getDefaultRootDevicePath(os providerconfigtypes.OperatingSystem) (string, error) {
	const (
		rootDevicePathSDA  = "/dev/sda1"
		rootDevicePathXVDA = "/dev/xvda"
	)

	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemCentOS:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemRockyLinux:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemRHEL:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemFlatcar:
		return rootDevicePathXVDA, nil
	case providerconfigtypes.OperatingSystemAmazonLinux2:
		return rootDevicePathXVDA, nil
	}

	return "", fmt.Errorf("no default root path found for %s operating system", os)
}

//gocyclo:ignore
func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *awstypes.RawConfig, error) {
	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := awstypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	c := Config{}
	c.AccessKeyID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeyID, "AWS_ACCESS_KEY_ID")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"accessKeyId\" field, error = %w", err)
	}
	c.SecretAccessKey, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.SecretAccessKey, "AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"secretAccessKey\" field, error = %w", err)
	}
	c.Region, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Region)
	if err != nil {
		return nil, nil, nil, err
	}
	c.VpcID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VpcID)
	if err != nil {
		return nil, nil, nil, err
	}
	c.SubnetID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.SubnetID)
	if err != nil {
		return nil, nil, nil, err
	}
	c.AvailabilityZone, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.AvailabilityZone)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, securityGroupIDRaw := range rawConfig.SecurityGroupIDs {
		securityGroupID, err := p.configVarResolver.GetConfigVarStringValue(securityGroupIDRaw)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SecurityGroupIDs = append(c.SecurityGroupIDs, securityGroupID)
	}
	c.InstanceProfile, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceProfile)
	if err != nil {
		return nil, nil, nil, err
	}

	instanceTypeStr, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceType)
	if err != nil {
		return nil, nil, nil, err
	}

	c.InstanceType = ec2types.InstanceType(instanceTypeStr)

	c.AMI, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.AMI)
	if err != nil {
		return nil, nil, nil, err
	}
	c.DiskSize = rawConfig.DiskSize
	diskTypeStr, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.DiskType)
	if err != nil {
		return nil, nil, nil, err
	}
	c.DiskType = ec2types.VolumeType(diskTypeStr)

	if c.DiskType == ec2types.VolumeTypeIo1 {
		if rawConfig.DiskIops == nil {
			return nil, nil, nil, errors.New("Missing required field `diskIops`")
		}
		iops := *rawConfig.DiskIops

		if iops < 100 || iops > 64000 {
			return nil, nil, nil, errors.New("Invalid value for `diskIops` (min: 100, max: 64000)")
		}

		c.DiskIops = rawConfig.DiskIops
	} else if c.DiskType == ec2types.VolumeTypeGp3 && rawConfig.DiskIops != nil {
		// gp3 disks start with 3000 IOPS by default, we _can_ pass better IOPS, but it is not a required field
		iops := *rawConfig.DiskIops

		if iops < 3000 || iops > 64000 {
			return nil, nil, nil, errors.New("Invalid value for `diskIops` (min: 3000, max: 64000)")
		}

		c.DiskIops = rawConfig.DiskIops
	}

	c.EBSVolumeEncrypted, _, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.EBSVolumeEncrypted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get ebsVolumeEncrypted value: %w", err)
	}
	c.Tags = rawConfig.Tags
	c.AssignPublicIP = rawConfig.AssignPublicIP
	c.IsSpotInstance = rawConfig.IsSpotInstance
	if rawConfig.SpotInstanceConfig != nil && c.IsSpotInstance != nil && *c.IsSpotInstance {
		maxPrice, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.SpotInstanceConfig.MaxPrice)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotMaxPrice = pointer.String(maxPrice)

		persistentRequest, _, err := p.configVarResolver.GetConfigVarBoolValue(rawConfig.SpotInstanceConfig.PersistentRequest)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotPersistentRequest = pointer.Bool(persistentRequest)

		interruptionBehavior, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.SpotInstanceConfig.InterruptionBehavior)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotInterruptionBehavior = pointer.String(interruptionBehavior)
	}
	assumeRoleARN, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AssumeRoleARN, "AWS_ASSUME_ROLE_ARN")
	if err != nil {
		return nil, nil, nil, err
	}
	c.AssumeRoleARN = assumeRoleARN
	assumeRoleExternalID, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AssumeRoleExternalID, "AWS_ASSUME_ROLE_EXTERNAL_ID")
	if err != nil {
		return nil, nil, nil, err
	}
	c.AssumeRoleExternalID = assumeRoleExternalID

	return &c, pconfig, rawConfig, err
}

func getAwsConfig(ctx context.Context, id, secret, token, region, assumeRoleARN, assumeRoleExternalID string) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(id, secret, token)),
		awsconfig.WithRetryMaxAttempts(maxRetries),
	)

	if err != nil {
		return aws.Config{}, err
	}

	if assumeRoleARN != "" {
		stsSvc := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsSvc, assumeRoleARN,
			func(o *stscreds.AssumeRoleOptions) {
				o.ExternalID = pointer.String(assumeRoleExternalID)
			},
		)

		cfg.Credentials = creds
	}

	return cfg, nil
}

func getEC2client(ctx context.Context, id, secret, region, assumeRoleArn, assumeRoleExternalID string) (*ec2.Client, error) {
	cfg, err := getAwsConfig(ctx, id, secret, "", region, assumeRoleArn, assumeRoleExternalID)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to get aws configuration")
	}

	return ec2.NewFromConfig(cfg), nil
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, _, rawConfig, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, err
	}
	if rawConfig.DiskType.Value == "" {
		rawConfig.DiskType.Value = string(ec2types.VolumeTypeStandard)
	}
	if rawConfig.AssignPublicIP == nil {
		rawConfig.AssignPublicIP = aws.Bool(true)
	}
	if rawConfig.IsSpotInstance != nil && *rawConfig.IsSpotInstance {
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		spec.Labels["k8c.io/aws-spot"] = "aws-node-termination-handler"
	}
	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	return spec, err
}

func (p *provider) Validate(ctx context.Context, spec clusterv1alpha1.MachineSpec) error {
	config, pc, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if _, osSupported := amiFilters[pc.OperatingSystem]; !osSupported {
		return fmt.Errorf("unsupported os %s", pc.OperatingSystem)
	}

	if _, ok := volumeTypes[config.DiskType]; !ok {
		return fmt.Errorf("invalid volume type %s specified. Supported: %s", config.DiskType, volumeTypes)
	}

	if config.InstanceType == "" {
		return fmt.Errorf("instanceType must be specified")
	}

	// Not the best test as the minimum disk size depends on the AMI
	// but the best we can do here
	if config.DiskSize == 0 {
		return fmt.Errorf("diskSize must be specified and > 0")
	}

	ec2Client, err := getEC2client(ctx, config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return fmt.Errorf("failed to create ec2 client: %w", err)
	}
	if config.AMI != "" {
		_, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			ImageIds: []string{config.AMI},
		})
		if err != nil {
			return fmt.Errorf("failed to validate ami: %w", err)
		}
	}

	vpc, err := getVpc(ctx, ec2Client, config.VpcID)
	if err != nil {
		return fmt.Errorf("invalid vpc %q specified: %w", config.VpcID, err)
	}

	switch f := pc.Network.GetIPFamily(); f {
	case util.IPFamilyUnspecified, util.IPFamilyIPv4:
		// noop
	case util.IPFamilyIPv6, util.IPFamilyIPv4IPv6, util.IPFamilyIPv6IPv4:
		if len(vpc.Ipv6CidrBlockAssociationSet) == 0 {
			return fmt.Errorf("vpc %s does not have IPv6 CIDR block", pointer.StringDeref(vpc.VpcId, ""))
		}
	default:
		return fmt.Errorf(util.ErrUnknownNetworkFamily, f)
	}

	_, err = ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{ZoneNames: []string{config.AvailabilityZone}})
	if err != nil {
		return fmt.Errorf("invalid zone %q specified: %w", config.AvailabilityZone, err)
	}

	_, err = ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{RegionNames: []string{config.Region}})
	if err != nil {
		return fmt.Errorf("invalid region %q specified: %w", config.Region, err)
	}

	if len(config.SecurityGroupIDs) == 0 {
		return errors.New("no security groups were specified")
	}
	_, err = ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: config.SecurityGroupIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to validate security group id's: %w", err)
	}

	if config.InstanceProfile == "" {
		return errors.New("no instance profile specified")
	}

	if config.IsSpotInstance != nil && *config.IsSpotInstance {
		if config.SpotMaxPrice == nil {
			return errors.New("failed to validate max price for the spot instance: max price cannot be empty when spot instance ")
		}
	}

	return nil
}

func getVpc(ctx context.Context, client *ec2.Client, id string) (*ec2types.Vpc, error) {
	vpcOut, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{id}},
		},
	})

	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to list vpc's")
	}

	if len(vpcOut.Vpcs) != 1 {
		return nil, fmt.Errorf("unable to find specified vpc with id %q", id)
	}

	return &vpcOut.Vpcs[0], nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(ctx, config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return nil, err
	}

	rootDevicePath, err := getDefaultRootDevicePath(pc.OperatingSystem)
	if err != nil {
		return nil, err
	}

	amiID := config.AMI
	if amiID == "" {
		// read the instance type to know which cpu architecture is needed in the AMI
		cpuArchitecture, err := getCPUArchitecture(ctx, ec2Client, config.InstanceType)

		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Failed to find instance type %s in region %s: %v", config.InstanceType, config.Region, err),
			}
		}

		if amiID, err = getDefaultAMIID(ctx, ec2Client, pc.OperatingSystem, config.Region, cpuArchitecture); err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Failed to get AMI-ID for operating system %s in region %s: %v", pc.OperatingSystem, config.Region, err),
			}
		}
	}

	if pc.OperatingSystem != providerconfigtypes.OperatingSystemFlatcar {
		// Gzip the userdata in case we don't use Flatcar
		userdata, err = convert.GzipString(userdata)
		if err != nil {
			return nil, fmt.Errorf("failed to gzip the userdata")
		}
	}

	tags := []ec2types.Tag{
		{
			Key:   aws.String(nameTag),
			Value: aws.String(machine.Spec.Name),
		},
		{
			Key:   aws.String(machineUIDTag),
			Value: aws.String(string(machine.UID)),
		},
	}

	for k, v := range config.Tags {
		tags = append(tags, ec2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	var instanceMarketOptions *ec2types.InstanceMarketOptionsRequest
	if config.IsSpotInstance != nil && *config.IsSpotInstance {
		spotOpts := &ec2types.SpotMarketOptions{
			SpotInstanceType: ec2types.SpotInstanceTypeOneTime,
		}

		if config.SpotMaxPrice != nil && *config.SpotMaxPrice != "" {
			spotOpts.MaxPrice = config.SpotMaxPrice
		}

		if config.SpotPersistentRequest != nil && *config.SpotPersistentRequest {
			spotOpts.SpotInstanceType = ec2types.SpotInstanceTypePersistent
			spotOpts.InstanceInterruptionBehavior = ec2types.InstanceInterruptionBehaviorStop

			if config.SpotInterruptionBehavior != nil && *config.SpotInterruptionBehavior != "" {
				spotOpts.InstanceInterruptionBehavior = ec2types.InstanceInterruptionBehavior(*config.SpotInterruptionBehavior)
			}
		}

		instanceMarketOptions = &ec2types.InstanceMarketOptionsRequest{
			MarketType:  ec2types.MarketTypeSpot,
			SpotOptions: spotOpts,
		}
	}

	// By default we assign a public IP - We introduced this field later, so we made it a pointer & default to true.
	// This must be done aside from the webhook defaulting as we might have machines which don't get defaulted before this
	assignPublicIP := config.AssignPublicIP == nil || *config.AssignPublicIP

	instanceRequest := &ec2.RunInstancesInput{
		ImageId:               aws.String(amiID),
		InstanceMarketOptions: instanceMarketOptions,
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: aws.String(rootDevicePath),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize:          aws.Int32(config.DiskSize),
					DeleteOnTermination: aws.Bool(true),
					VolumeType:          config.DiskType,
					Iops:                config.DiskIops,
					Encrypted:           pointer.Bool(config.EBSVolumeEncrypted),
				},
			},
		},
		MaxCount:     aws.Int32(1),
		MinCount:     aws.Int32(1),
		InstanceType: config.InstanceType,
		UserData:     aws.String(base64.StdEncoding.EncodeToString([]byte(userdata))),
		Placement: &ec2types.Placement{
			AvailabilityZone: aws.String(config.AvailabilityZone),
		},
		NetworkInterfaces: []ec2types.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex:              aws.Int32(0), // eth0
				AssociatePublicIpAddress: aws.Bool(assignPublicIP),
				DeleteOnTermination:      aws.Bool(true),
				SubnetId:                 aws.String(config.SubnetID),
				Groups:                   config.SecurityGroupIDs,
			},
		},
		IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
			Name: aws.String(config.InstanceProfile),
		},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags:         tags,
			},
		},
	}

	if pc.Network.GetIPFamily().HasIPv6() {
		instanceRequest.NetworkInterfaces[0].Ipv6AddressCount = aws.Int32(1)
	}

	runOut, err := ec2Client.RunInstances(ctx, instanceRequest)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed create instance at aws")
	}

	if err = p.waitForInstance(ctx, machine); err != nil {
		return nil, awsErrorToTerminalError(err, "failed provision instance at aws")
	}

	return &awsInstance{instance: &runOut.Instances[0]}, nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	ec2instance, err := p.get(ctx, machine)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return true, nil
		}
		return false, err
	}

	// (*Config, *providerconfigtypes.Config, *awstypes.RawConfig, error)
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)

	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(ctx, config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return false, err
	}

	if config.IsSpotInstance != nil && *config.IsSpotInstance &&
		config.SpotPersistentRequest != nil && *config.SpotPersistentRequest {
		cOut, err := ec2Client.CancelSpotInstanceRequests(ctx, &ec2.CancelSpotInstanceRequestsInput{
			SpotInstanceRequestIds: []string{*ec2instance.instance.SpotInstanceRequestId},
		})

		if err != nil {
			return false, awsErrorToTerminalError(err, "failed to cancel spot instance request")
		}

		if cOut.CancelledSpotInstanceRequests[0].State == ec2types.CancelSpotInstanceRequestStateCancelled {
			klog.V(3).Infof("successfully canceled spot instance request %s at aws", *ec2instance.instance.SpotInstanceRequestId)
		}
	}

	tOut, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{ec2instance.ID()},
	})
	if err != nil {
		return false, awsErrorToTerminalError(err, "failed to terminate instance")
	}

	if tOut.TerminatingInstances[0].PreviousState.Name != tOut.TerminatingInstances[0].CurrentState.Name {
		klog.V(3).Infof("successfully triggered termination of instance %s at aws", ec2instance.ID())
	}

	return false, nil
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	return p.get(ctx, machine)
}

func (p *provider) get(ctx context.Context, machine *clusterv1alpha1.Machine) (*awsInstance, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(ctx, config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return nil, err
	}

	inOut, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:" + machineUIDTag),
				Values: []string{string(machine.UID)},
			},
		},
	})
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to list instances from aws")
	}

	// We might have multiple instances (Maybe some old, terminated ones)
	// Thus we need to find the instance which is not in the terminated state
	for _, reservation := range inOut.Reservations {
		for _, i := range reservation.Instances {
			if i.State == nil || i.State.Name == ec2types.InstanceStateNameTerminated {
				continue
			}

			return &awsInstance{
				instance: &i,
			}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %w", err)
	}

	cc := &awstypes.CloudConfig{
		Global: awstypes.GlobalOpts{
			VPC:      c.VpcID,
			SubnetID: c.SubnetID,
			Zone:     c.AvailabilityZone,
		},
	}

	s, err := awstypes.CloudConfigToString(cc)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert cloud-config to string: %w", err)
	}

	return s, "aws", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = string(c.InstanceType)
		labels["region"] = c.Region
		labels["az"] = c.AvailabilityZone
		labels["ami"] = c.AMI
	}

	return labels, err
}

func (p *provider) MigrateUID(ctx context.Context, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	machineInstance, err := p.get(ctx, machine)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return nil
		}
		return fmt.Errorf("failed to get instance: %w", err)
	}

	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(ctx, config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return fmt.Errorf("failed to get EC2 client: %w", err)
	}

	_, err = ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{machineInstance.ID()},
		Tags:      []ec2types.Tag{{Key: aws.String(machineUIDTag), Value: aws.String(string(newUID))}}})
	if err != nil {
		return fmt.Errorf("failed to update instance with new machineUIDTag: %w", err)
	}

	return nil
}

type awsInstance struct {
	instance *ec2types.Instance
}

func (d *awsInstance) Name() string {
	return getTagValue(nameTag, d.instance.Tags)
}

func (d *awsInstance) ID() string {
	return pointer.StringDeref(d.instance.InstanceId, "")
}

func (d *awsInstance) ProviderID() string {
	if d.instance.InstanceId == nil {
		return ""
	}
	if d.instance.Placement.AvailabilityZone == nil {
		return "aws:///" + *d.instance.InstanceId
	}

	return "aws:///" + *d.instance.Placement.AvailabilityZone + "/" + *d.instance.InstanceId
}

func (d *awsInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{
		pointer.StringDeref(d.instance.PublicIpAddress, ""):  v1.NodeExternalIP,
		pointer.StringDeref(d.instance.PublicDnsName, ""):    v1.NodeExternalDNS,
		pointer.StringDeref(d.instance.PrivateIpAddress, ""): v1.NodeInternalIP,
		pointer.StringDeref(d.instance.PrivateDnsName, ""):   v1.NodeInternalDNS,
	}

	for _, netInterface := range d.instance.NetworkInterfaces {
		for _, addr := range netInterface.Ipv6Addresses {
			ipAddr := pointer.StringDeref(addr.Ipv6Address, "")

			// link-local addresses not very useful in machine status
			// filter them out
			if !util.IsLinkLocal(ipAddr) {
				addresses[ipAddr] = v1.NodeExternalIP
			}
		}
	}

	delete(addresses, "")

	return addresses
}

func (d *awsInstance) Status() instance.Status {
	switch d.instance.State.Name {
	case ec2types.InstanceStateNameRunning:
		return instance.StatusRunning
	case ec2types.InstanceStateNamePending:
		return instance.StatusCreating
	case ec2types.InstanceStateNameTerminated:
		return instance.StatusDeleted
	case ec2types.InstanceStateNameShuttingDown:
		return instance.StatusDeleting
	default:
		return instance.StatusUnknown
	}
}

func getTagValue(name string, tags []ec2types.Tag) string {
	for _, t := range tags {
		if *t.Key == name {
			return *t.Value
		}
	}
	return ""
}

// awsErrorToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus
//
// if the given error doesn't qualify the error passed as
// an argument will be formatted according to msg and returned.
func awsErrorToTerminalError(err error, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s, due to %w", msg, err)
	}

	if err != nil {
		var aerr smithy.APIError
		if !errors.As(err, &aerr) {
			return prepareAndReturnError()
		}
		switch aerr.ErrorCode() {
		case "InstanceLimitExceeded":
			return cloudprovidererrors.TerminalError{
				Reason:  common.InsufficientResourcesMachineError,
				Message: "You've reached the AWS quota for number of instances of this type",
			}
		case "AuthFailure":
			// authorization primitives come from MachineSpec
			// thus we are setting InvalidConfigurationMachineError
			return cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: "A request has been rejected due to invalid credentials which were taken from the MachineSpec",
			}
		case "OptInRequired":
			// User has to accept the terms of the AMI
			return cloudprovidererrors.TerminalError{
				Reason:  "AMI terms not accepted",
				Message: err.Error(),
			}
		default:
			return prepareAndReturnError()
		}
	}
	return nil
}

func setProviderSpec(rawConfig awstypes.RawConfig, provSpec clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if provSpec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, err
	}

	rawCloudProviderSpec, err := json.Marshal(rawConfig)
	if err != nil {
		return nil, err
	}

	pconfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCloudProviderSpec}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{Raw: rawPconfig}, nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	ctx := context.Background()

	metricInstancesForMachines.Reset()

	if len(machines.Items) < 1 {
		return nil
	}

	type ec2Credentials struct {
		accessKeyID          string
		secretAccessKey      string
		region               string
		assumeRoleARN        string
		assumeRoleExternalID string
	}

	var machineErrors []error
	machineEc2Credentials := map[string]ec2Credentials{}
	for _, machine := range machines.Items {
		config, _, _, err := p.getConfig(machines.Items[0].Spec.ProviderSpec)
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to parse MachineSpec of machine %s/%s, due to %w", machine.Namespace, machine.Name, err))
			continue
		}

		// Very simple and very stupid
		machineEc2Credentials[fmt.Sprintf("%s/%s/%s/%s/%s", config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)] = ec2Credentials{
			accessKeyID:          config.AccessKeyID,
			secretAccessKey:      config.SecretAccessKey,
			region:               config.Region,
			assumeRoleARN:        config.AssumeRoleARN,
			assumeRoleExternalID: config.AssumeRoleExternalID,
		}
	}

	allReservations := []ec2types.Reservation{}
	for _, cred := range machineEc2Credentials {
		ec2Client, err := getEC2client(ctx, cred.accessKeyID, cred.secretAccessKey, cred.region, cred.assumeRoleARN, cred.assumeRoleExternalID)
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to get EC2 client: %w", err))
			continue
		}
		inOut, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to get EC2 instances: %w", err))
			continue
		}
		allReservations = append(allReservations, inOut.Reservations...)
	}

	for _, machine := range machines.Items {
		metricInstancesForMachines.WithLabelValues(fmt.Sprintf("%s/%s", machine.Namespace, machine.Name)).Set(
			getInstanceCountForMachine(machine, allReservations))
	}

	if len(machineErrors) > 0 {
		return fmt.Errorf("errors: %v", machineErrors)
	}

	return nil
}

func getInstanceCountForMachine(machine clusterv1alpha1.Machine, reservations []ec2types.Reservation) float64 {
	var count float64
	for _, reservation := range reservations {
		for _, i := range reservation.Instances {
			if i.State == nil || i.State.Name == ec2types.InstanceStateNameTerminated {
				continue
			}

			for _, tag := range i.Tags {
				if *tag.Key != machineUIDTag {
					continue
				}

				if *tag.Value == string(machine.UID) {
					count = count + 1
				}
				break
			}
		}
	}
	return count
}

func filterSupportedRHELImages(images []ec2types.Image) ([]ec2types.Image, error) {
	var filteredImages []ec2types.Image
	for _, image := range images {
		if strings.HasPrefix(*image.Name, "RHEL-8") {
			filteredImages = append(filteredImages, image)
		}
	}

	if filteredImages == nil {
		return nil, errors.New("rhel 8 images are not found")
	}

	return filteredImages, nil
}

// waitForInstance waits for AWS instance to be created.
// If machine-controller tries to get an instance before it's fully-created,
// but after the instance request has been issued, it could
// happen that it detects that there's no instance and create it again.
// That could result in two or more instances created for one Machine object.
// This happens more often in some AWS regions because some regions have
// slower instance creation (e.g. us-east-1 and us-west-2).
func (p *provider) waitForInstance(ctx context.Context, machine *clusterv1alpha1.Machine) error {
	return wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		_, err := p.get(ctx, machine)
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			// Retry if instance is not found
			return false, nil
		} else if err != nil {
			// If it's any error other then InstanceNotFound,
			// return the error and stop retrying.
			return false, err
		}

		return true, nil
	})
}
