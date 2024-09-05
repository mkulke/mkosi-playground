# mkosi playground

Building azure images with mkosi

## Requirements

nix w/ flakes support (https://nixos.org/nix/)

## Build

### Enter development environment

```bash
nix develop
```

### Create secure boot key

```bash
mkosi genkey
```

### Build raw image

```bash
mkosi -C ./initrd build
mkosi build
```

### Test in qemu

Use root for KVM acceleration

```bash
sudo $(which mkosi) qemu
``` 

## Publish to Azure

### Convert secure boot certificate

```bash
openssl x509 -in mkosi.crt -out additionalsignature.der -outform DER
base64 -w0 additionalsignature.der
```

### Edit uplosi.conf

Populate the `uplosi.conf` values according to the image gallery and image definition. Put the base64 encoded certificate string into the `additionalSignatures` list.

Note: The image definition has to support trusted launch.

### Publish

```bash
uplosi upload image.raw
```

## Deploy

```bash
cd launch-vm
go mod tidy
go build
./launch-vm -h
```
