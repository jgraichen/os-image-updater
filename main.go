package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/creasty/defaults"
	"github.com/davecgh/go-spew/spew"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/imageimport"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

type tImage struct {
	ImageURL          string            `yaml:"image_url,omitempty"`
	ChecksumAlgorithm string            `yaml:"checksums_algo,omitempty" default:"sha256"`
	ChecksumURL       string            `yaml:"checksums_url,omitempty"`
	Properties        map[string]string `yaml:"properties,omitempty"`
	DiskFormat        string            `yaml:"disk_format,omitempty" default:"qcow2"`
	ContainerFormat   string            `yaml:"container_format,omitempty" default:"bare"`
}

type tConfig struct {
	Debug  bool              `yaml:"-"`
	DryRun bool              `yaml:"-"`
	Images map[string]tImage `yaml:"images,omitempty"`
}

type tChecksum struct {
	Algorithm string
	Value     string
}

var client *gophercloud.ServiceClient
var config *tConfig
var abort bool

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		abort = true
	}()

	config = &tConfig{}
	flag.BoolVar(&config.Debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&config.DryRun, "dryrun", false, "Do not perform changing actions")
	flag.Parse()

	var err error
	client, err = clientconfig.NewServiceClient("image", &clientconfig.ClientOpts{})
	if err != nil {
		log.Fatal(err)
	}

	content, err := ioutil.ReadFile("images.yml")
	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		log.Fatal(err)
	}

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	for name, conf := range config.Images {
		logger := log.WithField("image", name)

		defaults.Set(&conf)
		log.Debugf("Image configuration %s", spew.Sdump(conf))

		err := process(logger, name, conf)
		if err != nil {
			logger.Fatal(err)
		}

		if abort {
			break
		}
	}
}

func process(log *log.Entry, name string, image tImage) (err error) {
	log.Info("Fetch image details...")

	onError := func(f func()) {
		if err != nil {
			f()
		}
	}

	log.Debug("Getting remote image details...")
	osImages, err := findImages(name)
	if err != nil {
		return err
	}

	log.Debug("Getting remote checksum details...")
	md5, err := findMD5Checksum(image, name)
	if err != nil {
		return err
	}

	log.Debugf("Found %d images matching name", len(osImages))

	for _, osImage := range osImages {
		if osImage.Checksum == md5 {
			log.WithField("id", osImage.ID).Info("Checksum matches. Skip.")
			return nil
		}

		log.WithField("id", osImage.ID).Debug("Checksum match failed.")
	}

	opts := images.CreateOpts{
		Name:            name,
		DiskFormat:      image.DiskFormat,
		ContainerFormat: image.ContainerFormat,
		Properties:      image.Properties,
		Visibility:      visptr(images.ImageVisibilityPrivate),
	}

	log.Debugf("Image create opts: %s", spew.Sdump(opts))

	if config.DryRun {
		log.Info("Would create and import new image. Skip.")
		return nil
	}

	newImage, err := images.Create(client, &opts).Extract()
	if err != nil {
		return err
	}

	defer onError(func() {
		log.WithField("id", newImage.ID).Debugf("Rollback image resource...")
		err := images.Delete(client, newImage.ID).ExtractErr()
		if err != nil {
			log.Error(err)
		}
	})

	err = imageimport.Create(client, newImage.ID, &imageimport.CreateOpts{
		Name: imageimport.WebDownloadMethod,
		URI:  image.ImageURL,
	}).ExtractErr()

	if err != nil {
		return err
	}

	log.Info("Waiting for import to complete...")

	err = retry(func() error {
		newImage, err = images.Get(client, newImage.ID).Extract()
		if err != nil {
			log.Debug(err)
			return err
		}

		if newImage.Status != images.ImageStatusActive {
			log.WithField("id", newImage.ID).Debugf("Image status is %s", newImage.Status)
			return fmt.Errorf("Got images status %s, expected active", newImage.Status)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Publish new image...")

	_, err = images.Update(client, newImage.ID, images.UpdateOpts{
		images.UpdateVisibility{
			Visibility: images.ImageVisibilityPublic,
		},
	}).Extract()

	if err != nil {
		return err
	}

	if len(osImages) > 0 {
		log.Info("Delete old images...")

		for _, osImage := range osImages {
			err = images.Delete(client, osImage.ID).ExtractErr()
			if err != nil {
				log.Error(err)
			}
		}
	}

	return nil
}

func findImages(name string) ([]images.Image, error) {
	pages, err := images.List(client, &images.ListOpts{Name: name}).AllPages()
	if err != nil {
		log.Fatal(err)
	}

	images, err := images.ExtractImages(pages)
	if err != nil {
		log.Fatal(err)
	}

	return images, nil
}

func findMD5Checksum(conf tImage, name string) (string, error) {
	var ck string

	resp, err := http.Get(conf.ChecksumURL)
	if err != nil {
		return ck, err
	}

	defer resp.Body.Close()

	filename := filepath.Base(conf.ImageURL)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())

		if len(parts) < 1 {
			continue
		}

		if len(parts) == 2 {
			// Format: "<checksum> <file>"
			if parts[1] == filename {
				return parts[0], nil
			}
		} else if len(parts) == 3 {
			// Format: "<algo> <checksum> <file>"
			if parts[2] == filename && strings.Trim(parts[0], ":") == "md5" {
				return parts[1], nil
			}
		}
	}

	return "", nil
}

func retry(f func() error) error {
	var err error

	for i := 0; i < 150; i++ {
		time.Sleep(2 * time.Second)

		if abort {
			return errors.New("Action aborted")
		}

		if err = f(); err == nil {
			return nil
		}
	}

	return err
}

func visptr(v images.ImageVisibility) *images.ImageVisibility {
	return &v
}
