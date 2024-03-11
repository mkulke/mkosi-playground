# mkosi playground

Building azure images with mkosi

## Requirements

nix w/ flakes support (https://nixos.org/nix/)

## Usage

### Development environment

```bash
nix develop
```

### Build

```bash
mkosi -C ./initrd build
mkosi build
```

### Qemu

Use root for KVM acceleration

```bash
sudo $(which mkosi) qemu
``` 

### Publish to image gallery

```bash
make publish
```

### Deploy

```bash
cd launch-vm
go mod tidy
go build
./launch-vm
```
