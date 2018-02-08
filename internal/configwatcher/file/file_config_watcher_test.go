package file_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	. "github.com/solo-io/glue/internal/configwatcher/file"
	"github.com/solo-io/glue/pkg/api/types/v1"
	"github.com/solo-io/glue/pkg/configwatcher"
	"github.com/solo-io/glue/pkg/log"
	. "github.com/solo-io/glue/test/helpers"
)

type Spec struct {
	Params []Param
}

type Param struct {
	Name        string
	Description string
	Type        string
	Required    bool
}

var _ = Describe("FileConfigWatcher", func() {
	var (
		dir                         string
		err                         error
		watch                       configwatcher.Interface
		resourceDirs                = []string{"upstreams", "virtualhosts"}
		upstreamDir, virtualhostDir string
	)
	BeforeEach(func() {
		dir, err = ioutil.TempDir("", "filecachetest")
		Must(err)
		watch, err = NewFileConfigWatcher(dir, time.Millisecond)
		Must(err)
		upstreamDir = filepath.Join(dir, "upstreams")
		virtualhostDir = filepath.Join(dir, "virtualhosts")
	})
	AfterEach(func() {
		log.Debugf("removing " + dir)
		os.RemoveAll(dir)
	})
	Describe("init", func() {
		It("creates the expected subdirs", func() {
			files, err := ioutil.ReadDir(dir)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(2))
			var createdSubDirs []string
			for _, file := range files {
				Expect(file.IsDir()).To(BeTrue())
				createdSubDirs = append(createdSubDirs, filepath.Base(file.Name()))
			}
			for _, expectedSubDir := range resourceDirs {
				Expect(createdSubDirs).To(ContainElement(expectedSubDir))
			}
		})
	})
	Describe("watching directory", func() {
		Context("valid configs are written to the correct directories", func() {
			It("creates and updates configs for .yml or .yaml files found in the subdirs", func() {
				cfg := NewTestConfig()
				for _, us := range cfg.Upstreams {
					filename := fmt.Sprintf("%v.yaml", us.Name)
					err := writeConfigObjFile(us, upstreamDir, filename)
					Expect(err).NotTo(HaveOccurred())
				}
				for _, vhost := range cfg.VirtualHosts {
					filename := fmt.Sprintf("%v.yaml", vhost.Name)
					err := writeConfigObjFile(vhost, virtualhostDir, filename)
					Expect(err).NotTo(HaveOccurred())
				}
				var expectedCfg v1.Config
				data, err := json.Marshal(cfg)
				Expect(err).To(BeNil())
				err = json.Unmarshal(data, &expectedCfg)
				Expect(err).To(BeNil())
				Eventually(func() (v1.Config, error) { return readConfig(watch) }).Should(Equal(expectedCfg))
			})
		})
		Context("an invalid config is written to a dir", func() {
			It("sends an error on the Error() channel", func() {
				invalidConfig := []byte("wdf112 1`12")
				err = ioutil.WriteFile(filepath.Join(upstreamDir, "bad-upstream.yml"), invalidConfig, 0644)
				Expect(err).NotTo(HaveOccurred())
				select {
				case <-watch.Config():
					Fail("config was received, expected error")
				case err := <-watch.Error():
					Expect(err).To(HaveOccurred())
				case <-time.After(time.Second * 1):
					Fail("expected err to have occurred before 1s")
				}
			})
		})
	})
})

func writeConfigObjFile(v interface{}, dir, filename string) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(dir, filename), data, 0644)
}

var lastRead *v1.Config

func readConfig(watch configwatcher.Interface) (v1.Config, error) {
	select {
	case cfg := <-watch.Config():
		lastRead = cfg
		return *cfg, nil
	case err := <-watch.Error():
		return v1.Config{}, err
	case <-time.After(time.Second * 1):
		if lastRead != nil {
			return *lastRead, nil
		}
		return v1.Config{}, errors.New("expected new config to be read in before 1s")
	}
}
