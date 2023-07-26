# os-image-updater

A small program to keep cloud images in OpenStack up-to-date.

It uses a YAML file for configuration and checksums to skip not necessary updates. Previous images are deleted.

## Usage

```session
> ./os-image-updater --help
Usage of ./os-image-updater:
  -debug
      Enable debug logging
  -delete
      Delete old images instead of only changing visibility to private
  -dryrun
      Do not perform changing actions
  -filter string
      Only process images matching filter value
  -force
      Force uploading new image even if checksum matches
  -prefix string
      Prefix all image names
  -private
      Force image visibility to private
```

Configure access using environment variables. `OS_CLOUD` is supported.

## Example config

```yml
# images.yml
default: &default
  hw_disk_bus: scsi
  hw_qemu_guest_agent: yes
  hw_scsi_model: virtio-scsi
  img_hv_type: kvm
  os_require_quiesce: yes
  os_type: linux

images:
  debian-11:
    image_url: https://cloud.debian.org/images/cloud/bullseye/latest/debian-11-genericcloud-amd64.raw
    checksums_url: https://cloud.debian.org/images/cloud/bullseye/latest/SHA512SUMS
    disk_format: raw
    properties:
      <<: *default
      os_codename: bullseye
      os_distro: debian
      os_flavor: cloud
      os_version: 11

  ubuntu-20.04:
    image_url: https://cloud-images.ubuntu.com/releases/focal/release/ubuntu-20.04-server-cloudimg-amd64.img
    checksums_url: https://cloud-images.ubuntu.com/releases/focal/release/MD5SUMS
    properties:
      <<: *default
      os_codename: focal
      os_distro: ubuntu
      os_flavor: cloud
      os_version: 20.04
```

Only `images` is required.

`checksums_url` must be a file in one of the following formats:

* Checksum and filename:

  ```text
  123456789 cloudimage.qcow2
  ...
  ```

* Algorithm, checksum and filename:

  ```text
  md5: 123456789 cloudimgage.qcow2
  ...
  ```

* Or this format:

  ```text
  SHA256(cloudimgage.qcow2) = 123456789
  ...
  ```

The checksum is stored as a custom image property and only used to check if a new image needs to be imported.
