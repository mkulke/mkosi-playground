// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

const (
	location = "eastus"
)

var (
	resourceGroupName string
	subnetID          string
	imageID           string
	subscriptionID    string
	keep              bool
	sshPublicKeyPath  string
)

var (
	resourcesClientFactory *armresources.ClientFactory
	computeClientFactory   *armcompute.ClientFactory
	networkClientFactory   *armnetwork.ClientFactory
)

var (
	resourceGroupClient   *armresources.ResourceGroupsClient
	interfacesClient      *armnetwork.InterfacesClient
	virtualMachinesClient *armcompute.VirtualMachinesClient
	disksClient           *armcompute.DisksClient
)

type VMResources struct {
	vmName   string
	nicName  string
	diskName string
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "resource-group",
				Aliases:     []string{"g"},
				Usage:       "resource group name",
				EnvVars:     []string{"RESOURCE_GROUP"},
				Destination: &resourceGroupName,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "subnet-id",
				Aliases:     []string{"n"},
				Usage:       "subnet id to attach the nic to",
				EnvVars:     []string{"SUBNET_ID"},
				Destination: &subnetID,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "image-id",
				Aliases:     []string{"i"},
				Usage:       "image id to use for the virtual machine",
				EnvVars:     []string{"IMAGE_ID"},
				Destination: &imageID,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "subscription-id",
				Aliases:     []string{"s"},
				Usage:       "subscription id",
				EnvVars:     []string{"SUBSCRIPTION_ID"},
				Destination: &subscriptionID,
				Required:    true,
			},
			&cli.BoolFlag{
				Name:        "keep",
				Aliases:     []string{"k"},
				Usage:       "do not delete the resources after creating them",
				Destination: &keep,
			},
			&cli.StringFlag{
				Name:        "pub-key",
				Aliases:     []string{"p"},
				Usage:       "ssh public key path",
				EnvVars:     []string{"SSH_PUBLIC_KEY"},
				Destination: &sshPublicKeyPath,
			},
		},
		Action: func(cCtx *cli.Context) error {
			resources, err := createVM("test-vm")
			if err != nil {
				return err
			}
			if keep == true {
				return nil
			}
			return cleanup(resources)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func createVM(name string) (*VMResources, error) {
	conn, err := connectionAzure()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	networkClientFactory, err = armnetwork.NewClientFactory(subscriptionID, conn, nil)
	if err != nil {
		return nil, err
	}
	interfacesClient = networkClientFactory.NewInterfacesClient()

	computeClientFactory, err = armcompute.NewClientFactory(subscriptionID, conn, nil)
	if err != nil {
		return nil, err
	}
	virtualMachinesClient = computeClientFactory.NewVirtualMachinesClient()
	disksClient = computeClientFactory.NewDisksClient()

	log.Println("start creating virtual machine...")

	nicName := name + "-nic"
	nic, err := createNetWorkInterface(ctx, subnetID, nicName)
	if err != nil {
		return nil, err
	}
	log.Printf("created network interface: %s", *nic.ID)

	diskName := name + "-disk"
	virtualMachine, err := createVirtualMachine(ctx, *nic.ID, diskName, name)
	if err != nil {
		return nil, err
	}
	log.Printf("created network virtual machine: %s", *virtualMachine.ID)

	log.Println("virtual machine created successfully")

	return &VMResources{
		vmName:   name,
		nicName:  nicName,
		diskName: diskName,
	}, nil
}

func cleanup(resources *VMResources) error {
	ctx := context.Background()

	log.Println("start deleting virtual machine...")
	err := deleteVirtualMachine(ctx, resources.vmName)
	if err != nil {
		return err
	}
	log.Println("deleted virtual machine")

	err = deleteDisk(ctx, resources.diskName)
	if err != nil {
		return err
	}
	log.Println("deleted disk")

	err = deleteNetWorkInterface(ctx, resources.nicName)
	if err != nil {
		return err
	}
	log.Println("deleted network interface")

	log.Println("virtual machine deleted successfully")

	return nil
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}

func createNetWorkInterface(ctx context.Context, subnetID string, name string) (*armnetwork.Interface, error) {
	parameters := armnetwork.Interface{
		Location: to.Ptr(location),
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: to.Ptr("ipConfig"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: to.Ptr(subnetID),
						},
					},
				},
			},
		},
	}

	pollerResponse, err := interfacesClient.BeginCreateOrUpdate(ctx, resourceGroupName, name, parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.Interface, err
}

func deleteNetWorkInterface(ctx context.Context, name string) error {

	pollerResponse, err := interfacesClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func createVirtualMachine(ctx context.Context, networkInterfaceID string, diskName string, name string) (*armcompute.VirtualMachine, error) {
	//require ssh key for authentication on linux
	if sshPublicKeyPath == "" {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		sshPublicKeyPath = homedir + "/.ssh/id_rsa.pub"
	}
	var sshBytes []byte
	_, err := os.Stat(sshPublicKeyPath)
	if err != nil {
		return nil, err
	}
	sshBytes, err = os.ReadFile(sshPublicKeyPath)
	if err != nil {
		return nil, err
	}
	userName := "azureuser"

	securityProfile := &armcompute.SecurityProfile{
		SecurityType: to.Ptr(armcompute.SecurityTypesConfidentialVM),
		UefiSettings: &armcompute.UefiSettings{
			SecureBootEnabled: to.Ptr(false),
			VTpmEnabled:       to.Ptr(true),
		},
	}

	parameters := armcompute.VirtualMachine{
		Location: to.Ptr(location),
		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
		},
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					//CommunityGalleryImageID: to.Ptr(imageID),
					ID: to.Ptr(imageID),
				},
				OSDisk: &armcompute.OSDisk{
					Name:         to.Ptr(diskName),
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
					Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
					DeleteOption: to.Ptr(armcompute.DiskDeleteOptionTypesDelete),
					ManagedDisk: &armcompute.ManagedDiskParameters{
						StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS), // OSDisk type Standard/Premium HDD/SSD
						SecurityProfile: &armcompute.VMDiskSecurityProfile{
							SecurityEncryptionType: to.Ptr(armcompute.SecurityEncryptionTypesVMGuestStateOnly),
						},
					},
					DiskSizeGB: to.Ptr[int32](20), // default 127G
				},
			},
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes("Standard_DC2as_v5")),
			},
			OSProfile: &armcompute.OSProfile{ //
				ComputerName:  to.Ptr(name),
				AdminUsername: to.Ptr(userName),
				// 	AdminPassword: to.Ptr("Password01!@#"),
				//require ssh key for authentication on linux
				LinuxConfiguration: &armcompute.LinuxConfiguration{
					DisablePasswordAuthentication: to.Ptr(true),
					SSH: &armcompute.SSHConfiguration{
						// PublicKeys: []*armcompute.SSHPublicKey{},
						PublicKeys: []*armcompute.SSHPublicKey{
							{
								Path:    to.Ptr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", userName)),
								KeyData: to.Ptr(string(sshBytes)),
							},
						},
					},
				},
			},
			SecurityProfile: securityProfile,
			DiagnosticsProfile: &armcompute.DiagnosticsProfile{
				BootDiagnostics: &armcompute.BootDiagnostics{
					Enabled: to.Ptr(true),
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: to.Ptr(networkInterfaceID),
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							DeleteOption: to.Ptr(armcompute.DeleteOptionsDelete),
						},
					},
				},
			},
		},
	}

	pollerResponse, err := virtualMachinesClient.BeginCreateOrUpdate(ctx, resourceGroupName, name, parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualMachine, nil
}

func deleteVirtualMachine(ctx context.Context, name string) error {
	pollerResponse, err := virtualMachinesClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func deleteDisk(ctx context.Context, name string) error {

	pollerResponse, err := disksClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}
	return nil
}
