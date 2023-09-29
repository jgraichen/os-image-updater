package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/creasty/defaults"
	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/oriser/regroup"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	om "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"

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
	Visibility        string            `yaml:"visibility,omitempty" default:"public"`
	MinDisk           int               `yaml:"min_disk,omitempty" default:"0"`
	MinRAM            int               `yaml:"min_ram,omitempty" default:"0"`
}

type tConfig struct {
	Debug   bool                          `yaml:"-"`
	Delete  bool                          `yaml:"delete" default:"false"`
	DryRun  bool                          `yaml:"-"`
	Filter  string                        `yaml:"-"`
	Force   bool                          `yaml:"-"`
	Private bool                          `yaml:"-"`
	Prefix  string                        `yaml:"prefix"`
	Images  om.OrderedMap[string, tImage] `yaml:"images,omitempty"`
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
	flag.BoolVar(&config.Delete, "delete", false, "Delete old images instead of only changing visibility to private")
	flag.BoolVar(&config.Force, "force", false, "Force uploading new image even if checksum matches")
	flag.BoolVar(&config.Private, "private", false, "Force image visibility to private")
	flag.StringVar(&config.Filter, "filter", "", "Only process images matching filter value")
	flag.StringVar(&config.Prefix, "prefix", "", "Prefix all image names")
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

	filter := glob.MustCompile(config.Filter)

	for pair := config.Images.Oldest(); pair != nil; pair = pair.Next() {
		imageName := pair.Key
		if config.Prefix != "" {
			imageName = config.Prefix + pair.Key
		}

		logger := log.WithField("image", imageName)

		if len(config.Filter) > 0 && !filter.Match(pair.Key) {
			logger.Debug("Image does not match filter. Skip.")
			continue
		}

		defaults.Set(&pair.Value)
		log.Debugf("Image configuration %s", spew.Sdump(pair.Value))

		err := process(logger, imageName, pair.Value)
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
	osImages, err := findPublicImages(name)
	if err != nil {
		return err
	}

	log.Debug("Getting remote checksum details...")
	checksum, err := findChecksum(image, name)
	if err != nil {
		return err
	}

	log.Debugf("Found %d images matching name", len(osImages))

	for _, osImage := range osImages {
		logger := log.WithField("id", osImage.ID)

		value := osImage.Checksum
		if val, exists := osImage.Properties["int:original-checksum"]; exists {
			value = val.(string)
		}

		if value == checksum {
			if config.Force {
				logger.Info("Image up-to-date but forced to update image.")
				break
			} else {
				logger.Info("Image up-to-date. Skip.")
				return nil
			}
		}

		logger.WithFields(logrus.Fields{
			"expected": checksum,
			"got":      value,
		}).Debugf("Checksum match failed")
	}

	opts := images.CreateOpts{
		Name:            name,
		DiskFormat:      image.DiskFormat,
		ContainerFormat: image.ContainerFormat,
		Properties:      image.Properties,
		MinDisk:         image.MinDisk,
		MinRAM:          image.MinRAM,
		Visibility:      visptr(images.ImageVisibilityPrivate),
	}

	opts.Properties["int:original-checksum"] = checksum

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
			return fmt.Errorf("got images status %s, expected active", newImage.Status)
		}

		return nil
	})

	if err != nil {
		return err
	}

	if !config.Private && image.Visibility == "public" {
		log.Info("Publish new image...")

		_, err = images.Update(client, newImage.ID, images.UpdateOpts{
			images.UpdateVisibility{
				Visibility: images.ImageVisibilityPublic,
			},
		}).Extract()

		if err != nil {
			return err
		}
	}

	if len(osImages) > 0 {
		log.Info("Update old images to private...")

		for _, osImage := range osImages {
			_, err = images.Update(client, osImage.ID, images.UpdateOpts{
				images.UpdateVisibility{
					Visibility: images.ImageVisibilityPrivate,
				},
			}).Extract()
			if err != nil {
				log.Error(err)
			}
		}

		if config.Delete {
			log.Info("Delete old images...")

			for _, osImage := range osImages {
				err = images.Delete(client, osImage.ID).ExtractErr()
				if err != nil {
					log.Error(err)
				}
			}
		}
	}

	return nil
}

func findPublicImages(name string) ([]images.Image, error) {
	pages, err := images.List(client, &images.ListOpts{
		Name:       name,
		Visibility: images.ImageVisibilityPublic,
	}).AllPages()
	if err != nil {
		log.Fatal(err)
	}

	images, err := images.ExtractImages(pages)
	if err != nil {
		log.Fatal(err)
	}

	return images, nil
}

func findChecksum(conf tImage, name string) (string, error) {
	var ck string

	resp, err := http.Get(conf.ChecksumURL)
	if err != nil {
		return ck, err
	}

	defer resp.Body.Close()

	filename := filepath.Base(conf.ImageURL)
	return scanChecksum(filename, resp.Body)
}

type ChecksumMatch struct {
	Checksum string `regroup:"checksum"`
	Filename string `regroup:"filename"`
}

func scanChecksum(filename string, data io.Reader) (string, error) {
	scanner := bufio.NewScanner(data)
	patterns := make([]*regroup.ReGroup, 0, 3)

	// "CHECKSUM filename"
	patterns = append(patterns, regroup.MustCompile(`^\s*(?P<checksum>[A-Fa-f0-9]+)\s+\*?(?P<filename>.+)\s*$`))

	// "algo: CHECKSUM filename"
	patterns = append(patterns, regroup.MustCompile(`^\s*\w+:?\s*(?P<checksum>[A-Fa-f0-9]+)\s+\*?(?P<filename>.+)\s*$`))

	// "ALGO(filename) = CHECKSUM"
	patterns = append(patterns, regroup.MustCompile(`^\s*\w+\((?P<filename>.+)\)\s*=\s*(?P<checksum>[A-Fa-f0-9]+)\s*$`))

	for scanner.Scan() {
		text := scanner.Text()
		for _, regroup := range patterns {
			match := &ChecksumMatch{}
			regroup.MatchToTarget(text, match)

			if match.Filename == filename && match.Checksum != "" {
				return match.Checksum, nil
			}
		}
	}

	return "", nil
}

func scanChecksumPattern(regex *regexp.Regexp, filename string, text string) string {
	m := regex.FindStringSubmatch(text)

	if m != nil && m[3] == filename {
		return m[2]
	}

	return ""
}

func retry(f func() error) error {
	var err error

	for i := 0; i < 150; i++ {
		time.Sleep(2 * time.Second)

		if abort {
			return errors.New("action aborted")
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
