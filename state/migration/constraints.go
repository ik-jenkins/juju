// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// ConstraintsArgs is an argument struct to construct Constraints.
type ConstraintsArgs struct {
	Architecture string
	Container    string
	CpuCores     uint64
	CpuPower     uint64
	InstanceType string
	Memory       uint64
	RootDisk     uint64

	Spaces []string
	Tags   []string
	// TODO: confirm networks not needed
}

func newConstraints(args ConstraintsArgs) *constraints {
	tags := make([]string, len(args.Tags))
	copy(tags, args.Tags)
	spaces := make([]string, len(args.Spaces))
	copy(spaces, args.Spaces)
	return &constraints{
		Version:       1,
		Architecture_: args.Architecture,
		Container_:    args.Container,
		CpuCores_:     args.CpuCores,
		CpuPower_:     args.CpuPower,
		InstanceType_: args.InstanceType,
		Memory_:       args.Memory,
		RootDisk_:     args.RootDisk,
		Spaces_:       spaces,
		Tags_:         tags,
	}
}

type constraints struct {
	Version int `yaml:"version"`

	Architecture_ string `yaml:"architecture,omitempty"`
	Container_    string `yaml:"container,omitempty"`
	CpuCores_     uint64 `yaml:"cpu-cores,omitempty"`
	CpuPower_     uint64 `yaml:"cpu-power,omitempty"`
	InstanceType_ string `yaml:"instance-type,omitempty"`
	Memory_       uint64 `yaml:"memory,omitempty"`
	RootDisk_     uint64 `yaml:"root-disk,omitempty"`

	Spaces_ []string `yaml:"spaces,omitempty"`
	Tags_   []string `yaml:"tags,omitempty"`
}

// Architecture implements Constraints.
func (c *constraints) Architecture() string {
	return c.Architecture_
}

// Container implements Constraints.
func (c *constraints) Container() string {
	return c.Container_
}

// CpuCores implements Constraints.
func (c *constraints) CpuCores() uint64 {
	return c.CpuCores_
}

// CpuPower implements Constraints.
func (c *constraints) CpuPower() uint64 {
	return c.CpuPower_
}

// InstanceType implements Constraints.
func (c *constraints) InstanceType() string {
	return c.InstanceType_
}

// Memory implements Constraints.
func (c *constraints) Memory() uint64 {
	return c.Memory_
}

// RootDisk implements Constraints.
func (c *constraints) RootDisk() uint64 {
	return c.RootDisk_
}

// Spaces implements Constraints.
func (c *constraints) Spaces() []string {
	spaces := make([]string, len(c.Spaces_))
	copy(spaces, c.Spaces_)
	return spaces
}

// Tags implements Constraints.
func (c *constraints) Tags() []string {
	tags := make([]string, len(c.Tags_))
	copy(tags, c.Tags_)
	return tags
}

func importConstraints(source map[string]interface{}) (*constraints, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "constraints version schema check failed")
	}

	importFunc, ok := constraintsDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type constraintsDeserializationFunc func(map[string]interface{}) (*constraints, error)

var constraintsDeserializationFuncs = map[int]constraintsDeserializationFunc{
	1: importConstraintsV1,
}

func importConstraintsV1(source map[string]interface{}) (*constraints, error) {
	fields := schema.Fields{
		"architecture":  schema.String(),
		"container":     schema.String(),
		"cpu-cores":     schema.Uint(),
		"cpu-power":     schema.Uint(),
		"instance-type": schema.String(),
		"memory":        schema.Uint(),
		"root-disk":     schema.Uint(),

		"spaces": schema.List(schema.String()),
		"tags":   schema.List(schema.String()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"architecture":  "",
		"container":     "",
		"cpu-cores":     uint64(0),
		"cpu-power":     uint64(0),
		"instance-type": "",
		"memory":        uint64(0),
		"root-disk":     uint64(0),

		"spaces": schema.Omit,
		"tags":   schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "constraints v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	return &constraints{
		Version:       1,
		Architecture_: valid["architecture"].(string),
		Container_:    valid["container"].(string),
		CpuCores_:     valid["cpu-cores"].(uint64),
		CpuPower_:     valid["cpu-power"].(uint64),
		InstanceType_: valid["instance-type"].(string),
		Memory_:       valid["memory"].(uint64),
		RootDisk_:     valid["root-disk"].(uint64),

		Spaces_: convertToStringSlice(valid["spaces"]),
		Tags_:   convertToStringSlice(valid["tags"]),
	}, nil
}

type hasConstraints *constraints

// Constraints implements HasConstraints.
func (c *hasConstraints) Constraints() Constraints {
	if c == nil || *c == nil {
		return nil
	}
	return *a
}

// SetAnnotations implements HasConstraints.
func (c *hasConstraints) SetConstraints(args ConstraintsArgs) {
	*c = newConstraints(args)
}

func (c *hasConstraints) importConstraints(valid map[string]interface{}) error {
	if constraintsMap, ok := valid["constraints"]; ok {
		constraints, err := importConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return errors.Trace(err)
		}
		*c = constraints
	}
	return nil
}

func addAnnotationSchema(fields schema.Fields, defaults schema.Defaults) {
	fields["constraints"] = schema.StringMap(schema.Any())
	defaults["constraints"] = schema.Omit
}
