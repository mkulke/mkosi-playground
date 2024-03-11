AZURE_REGION ?= eastus
AZURE_RESOURCE_GROUP ?= resourcegroup
AZURE_STORAGE_ACCOUNT ?= eastusstorageaccount
AZURE_STORAGE_CONTAINER ?= mkosi
GALLERY_NAME ?= galleryname
GALLERY_IMAGE_DEF_NAME ?= imagedefinition
IMAGE_SIZE ?= 2G

ifndef GALLERY_IMAGE_VERSION
$(error GALLERY_IMAGE_VERSION is not set)
endif

VHD_FILE := image-$(GALLERY_IMAGE_VERSION).vhd
VHD_ARTIFACT := $(VHD_FILE)
VHD_URL := https://$(AZURE_STORAGE_ACCOUNT).blob.core.windows.net/$(AZURE_STORAGE_CONTAINER)/$(VHD_FILE)
BYTES := $(shell numfmt --from=iec $(IMAGE_SIZE))
MB := $$((1024 * 1024))
ROUNDED_SIZE := $$(($(BYTES) / $(MB) * $(MB)))
NOW_PLUS_1H := $(shell date -u -d '+1 hour' '+%Y-%m-%dT%H:%MZ')

image.raw:
	mkosi build

.PHONY: $(VHD_ARTIFACT)
$(VHD_ARTIFACT): image.raw
	qemu-img resize --shrink -f raw image.raw $(ROUNDED_SIZE) && \
	qemu-img convert -f raw -o subformat=fixed,force_size -O vpc image.raw $(VHD_ARTIFACT)

.PHONY: sas-token
sas-token:
	$(eval SAS_TOKEN := $(shell az storage blob generate-sas \
		--account-name $(AZURE_STORAGE_ACCOUNT) \
		--blob-url $(VHD_URL) \
		--permissions rw \
		--expiry $(NOW_PLUS_1H) \
		--https-only \
		--output tsv))

.PHONY: upload
upload: $(VHD_ARTIFACT) sas-token
	azcopy cp $(VHD_ARTIFACT) "$(VHD_URL)?$(SAS_TOKEN)"

.PHONY: publish
publish: upload
	az sig image-version create \
		--resource-group $(AZURE_RESOURCE_GROUP) \
		--gallery-name $(GALLERY_NAME) \
		--gallery-image-definition $(GALLERY_IMAGE_DEF_NAME) \
		--gallery-image-version $(GALLERY_IMAGE_VERSION) \
		--target-regions $(AZURE_REGION) \
		--os-vhd-uri $(VHD_URL) \
		--os-vhd-storage-account $(AZURE_STORAGE_ACCOUNT)
