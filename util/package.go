package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver"

	"gopkg.in/yaml.v2"
)

type Package struct {
	Name          string  `yaml:"name" json:"name"`
	Title         *string `yaml:"title,omitempty" json:"title,omitempty"`
	Version       string  `yaml:"version" json:"version"`
	versionSemVer semver.Version
	Description   string   `yaml:"description" json:"description"`
	Categories    []string `yaml:"categories" json:"categories"`
	Requirement   struct {
		Kibana struct {
			Min       string `yaml:"version.min" json:"version.min"`
			Max       string `yaml:"version.max" json:"version.max"`
			minSemVer semver.Version
			maxSemVer semver.Version
		} `yaml:"kibana" json:"kibana"`
	} `yaml:"requirement" json:"requirement"`
	Screenshots []Image  `yaml:"screenshots,omitempty" json:"screenshots,omitempty"`
	Icons       []Image  `yaml:"icons,omitempty" json:"icons,omitempty"`
	Assets      []string `yaml:"assets,omitempty" json:"assets,omitempty"`
}

type Image struct {
	Src   string `yaml:"src" json:"src,omitempty"`
	Title string `yaml:"title" json:"title,omitempty"`
	Size  string `yaml:"size" json:"size,omitempty"`
	Type  string `yaml:"type" json:"type,omitempty"`
}

func (i Image) getPath(p *Package) string {
	return "/package/" + p.Name + "-" + p.Version + i.Src
}

// NewPackage creates a new package instances based on the given base path + package name.
// The package name passed contains the version of the package.
func NewPackage(basePath, packageName string) (*Package, error) {

	manifest, err := ioutil.ReadFile(basePath + "/" + packageName + "/manifest.yml")
	if err != nil {
		return nil, err
	}

	var p = &Package{}
	err = yaml.Unmarshal(manifest, p)
	if err != nil {
		return nil, err
	}

	if p.Icons != nil {
		for k, i := range p.Icons {
			p.Icons[k].Src = i.getPath(p)
		}
	}

	if p.Screenshots != nil {
		for k, s := range p.Screenshots {
			p.Screenshots[k].Src = s.getPath(p)
		}
	}

	if p.Requirement.Kibana.Max != "" {
		p.Requirement.Kibana.maxSemVer, err = semver.Parse(p.Requirement.Kibana.Max)
		if err != nil {
			return nil, err
		}
	}

	if p.Requirement.Kibana.Min != "" {
		p.Requirement.Kibana.minSemVer, err = semver.Parse(p.Requirement.Kibana.Min)
		if err != nil {
			return nil, err
		}
	}

	p.versionSemVer, err = semver.Parse(p.Version)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Package) HasCategory(category string) bool {
	for _, c := range p.Categories {
		if c == category {
			return true
		}
	}

	return false
}

func (p *Package) HasKibanaVersion(version *semver.Version) bool {
	if version != nil {
		if p.Requirement.Kibana.Max != "" {
			if version.GT(p.Requirement.Kibana.maxSemVer) {
				return false
			}
		}

		if p.Requirement.Kibana.Min != "" {
			if version.LT(p.Requirement.Kibana.minSemVer) {
				return false
			}
		}
	}
	return true
}

func (p *Package) IsNewer(pp *Package) bool {
	return p.versionSemVer.GT(pp.versionSemVer)
}

// LoadAssets (re)loads all the assets of the package
// Based on the time when this is called, it might be that not all assets for a package exist yet, so it is reset every time.
func (p *Package) LoadAssets(packagePath string) (err error) {
	// Reset Assets
	p.Assets = nil

	oldDir, err := os.Getwd()
	if err != nil {
		return err
	}
	defer func() {
		// use named return to also have an error in case the defer fails
		err = os.Chdir(oldDir)
	}()
	err = os.Chdir(packagePath)
	if err != nil {
		return err
	}

	assets, err := filepath.Glob("*")
	if err != nil {
		return err
	}

	a, err := filepath.Glob("*/*")
	if err != nil {
		return err
	}
	assets = append(assets, a...)

	a, err = filepath.Glob("*/*/*")
	if err != nil {
		return err
	}
	assets = append(assets, a...)

	for _, a := range assets {
		// Unfortunately these files keep sneaking in
		if strings.Contains(a, ".DS_Store") {
			continue
		}

		info, err := os.Stat(a)
		if err != nil {
			return err
		}

		if info.IsDir() {
			continue
		}

		a = "/package/" + packagePath + "/" + a
		p.Assets = append(p.Assets, a)
	}
	return nil
}
