package birdcage

import (
	"bytes"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/k8sdeps"
	"sigs.k8s.io/kustomize/pkg/factory"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/loader"
	"sigs.k8s.io/kustomize/pkg/target"
	"sigs.k8s.io/kustomize/pkg/transformers/config"
)

const (
	inputFilename         = "/input.yaml"
	patchFilename         = "/patch.yaml"
	kustomizationFilename = "/kustomization.yaml"
)

var rawKustomization = fmt.Sprintf(`
resources:
- %s
patches:
- %s
`, inputFilename, patchFilename)

type KustomizeHelper struct {
	FileSystem        fs.FileSystem
	Factory           *factory.KustFactory
	TransformerConfig *config.TransformerConfig
	Serializer        runtime.Serializer
}

func NewKustomizeHelper(serializer runtime.Serializer) *KustomizeHelper {
	fileSystem := fs.MakeFakeFS()
	fileSystem.WriteFile(kustomizationFilename, []byte(rawKustomization))

	return &KustomizeHelper{
		FileSystem:        fileSystem,
		Factory:           k8sdeps.NewFactory(),
		TransformerConfig: config.NewFactory(nil).DefaultConfig(),
		Serializer:        serializer,
	}
}

func (k *KustomizeHelper) Patch(src *appsv1.Deployment, patch []byte) (*appsv1.Deployment, error) {
	rawInput, err := encodeObject(k.Serializer, src)
	if err != nil {
		return nil, err
	}
	k.FileSystem.WriteFile(inputFilename, rawInput)

	k.FileSystem.WriteFile(patchFilename, patch)

	ldr, err := loader.NewLoader("/", k.FileSystem)
	if err != nil {
		return nil, err
	}
	defer ldr.Cleanup()

	kt, err := target.NewKustTarget(ldr, k.FileSystem, k.Factory.ResmapF, k.Factory.TransformerF, k.TransformerConfig)
	if err != nil {
		return nil, err
	}
	allResources, err := kt.MakeCustomizedResMap()
	if err != nil {
		return nil, err
	}

	rawOut, err := allResources.EncodeAsYaml()
	if err != nil {
		return nil, err
	}

	gvk := src.GetObjectKind().GroupVersionKind()
	out := appsv1.Deployment{}
	_, _, err = k.Serializer.Decode(rawOut, &gvk, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func encodeObject(codec runtime.Encoder, obj runtime.Object) ([]byte, error) {
	b := bytes.Buffer{}

	err := codec.Encode(obj, &b)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
