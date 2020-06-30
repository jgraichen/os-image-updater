# os-image-updater

A small program to keep cloud images in OpenStack up-to-date.

It uses a YAML file for configuration and checksums to skip not necessary updates. Previous images are deleted.

## Usage

```
$ ./os-image-updater [--debug] [--dryrun]
```

Configure access using environment variables. `OS_CLOUD` is supported.

## Example config

```yml
# images.yml
default: &default
  hw_disk_bus: scsi
  hw_scsi_model: virtio-scsi
  img_hv_type: kvm
  os_type: linux

images:
  rancheros:
    image_url: https://github.com/rancher/os/releases/latest/download/rancheros-openstack.img
    checksums_url: https://github.com/rancher/os/releases/latest/download/checksums.txt
    properties:
      <<: *default
      os_codename: rancheros

  debian-10:
    image_url: https://cloud.debian.org/images/cloud/OpenStack/current-10/debian-10-openstack-amd64.qcow2
    checksums_url: https://cloud.debian.org/images/cloud/OpenStack/current-10/MD5SUMS
    properties:
      <<: *default
      os_codename: buster
      os_distro: debian
      os_flavor: cloud
      os_version: 10

  ubuntu-18.04:
    image_url: https://cloud-images.ubuntu.com/releases/bionic/release/ubuntu-18.04-server-cloudimg-amd64.img
    checksums_url: https://cloud-images.ubuntu.com/releases/bionic/release/MD5SUMS
    properties:
      <<: *default
      os_codename: bionic
      os_distro: ubuntu
      os_flavor: cloud
      os_version: 18.04
```

Only `images` is required.

`checksums_url` must be a file in one of the following formats:

* Checksum and filename:

      123456789 cloudimage.qcow2
      ...

* Algorithm, checksum and filename:

      md5: 123456789 cloudimgage.qcow2
      ...

Checksums must be MD5.
