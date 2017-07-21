package main

import (
	"encoding/json"
	"time"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libkpod/driver"
	"github.com/kubernetes-incubator/cri-o/libkpod/image"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

type imageData struct {
	ID              string
	Names           []string
	Digests         []digest.Digest
	Parent          string
	Comment         string
	Created         *time.Time
	Container       string
	ContainerConfig containerConfig
	Author          string
	Config          ociv1.ImageConfig
	Architecture    string
	OS              string
	Size            uint
	VirtualSize     uint
	GraphDriver     driverData
	RootFS          ociv1.RootFS
}

type containerConfig struct {
	Hostname     string
	Domainname   string
	User         string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Tty          bool
	OpenStdin    bool
	StdinOnce    bool
	Env          []string
	Cmd          []string
	ArgsEscaped  bool
	Image        digest.Digest
	Volumes      map[string]interface{}
	WorkingDir   string
	Entrypoint   []string
	Labels       interface{}
	OnBuild      []string
}

type rootFS struct {
	Type   string
	Layers []string
}

func getImageData(store storage.Store, name string) (*imageData, error) {
	img, err := findImage(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", name)
	}

	cid, err := openImage(store, name)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", name)
	}
	digests, err := getDigests(*img)
	if err != nil {
		return nil, err
	}

	var bigData interface{}
	ctrConfig := containerConfig{}
	container := ""
	if len(digests) > 0 {
		bd, err := store.ImageBigData(img.ID, string(digests[len(digests)-1]))
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bd, &bigData)
		if err != nil {
			return nil, err
		}

		container = (bigData.(map[string]interface{})["container"]).(string)
		cc, err := json.MarshalIndent((bigData.(map[string]interface{})["container_config"]).(map[string]interface{}), "", "    ")
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(cc, &ctrConfig)
		if err != nil {
			return nil, err
		}
	}

	driverName, err := driver.GetDriverName(store)
	if err != nil {
		return nil, err
	}

	topLayerID, err := image.GetTopLayerID(*img)
	if err != nil {
		return nil, err
	}
	driverMetadata, err := driver.GetDriverMetadata(store, topLayerID)
	if err != nil {
		return nil, err
	}

	lstore, err := store.LayerStore()
	if err != nil {
		return nil, err
	}
	layer, err := lstore.Get(topLayerID)
	if err != nil {
		return nil, err
	}
	size, err := lstore.DiffSize(layer.Parent, layer.ID)
	if err != nil {
		return nil, err
	}

	virtualSize, err := image.Size(store, *img)
	if err != nil {
		return nil, err
	}

	return &imageData{
		ID:              img.ID,
		Names:           img.Names,
		Digests:         digests,
		Parent:          string(cid.Docker.Parent),
		Comment:         cid.OCIv1.History[0].Comment,
		Created:         cid.OCIv1.Created,
		Container:       container,
		ContainerConfig: ctrConfig,
		Author:          cid.OCIv1.Author,
		Config:          cid.OCIv1.Config,
		Architecture:    cid.OCIv1.Architecture,
		OS:              cid.OCIv1.OS,
		Size:            uint(size),
		VirtualSize:     uint(virtualSize),
		GraphDriver: driverData{
			Name: driverName,
			Data: driverMetadata,
		},
		RootFS: cid.OCIv1.RootFS,
	}, nil
}

func getDigests(img storage.Image) ([]digest.Digest, error) {
	metadata, err := image.ParseMetadata(img)
	if err != nil {
		return nil, err
	}
	digests := []digest.Digest{}
	for _, blob := range metadata.Blobs {
		digests = append(digests, blob.Digest)
	}
	return digests, nil
}
