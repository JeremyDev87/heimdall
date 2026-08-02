package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	assets "github.com/JeremyDev87/heimdall"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func Validate(name string, document any) error {
	schemaBytes, err := assets.FS.ReadFile("schemas/" + name)
	if err != nil {
		return fmt.Errorf("schema unavailable: %w", err)
	}
	var schemaDocument any
	decoder := json.NewDecoder(bytes.NewReader(schemaBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&schemaDocument); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	location := "https://heimdall.local/schemas/" + name
	if err := compiler.AddResource(location, schemaDocument); err != nil {
		return err
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return err
	}
	documentBytes, err := json.Marshal(document)
	if err != nil {
		return err
	}
	documentDecoder := json.NewDecoder(bytes.NewReader(documentBytes))
	documentDecoder.UseNumber()
	var normalized any
	if err := documentDecoder.Decode(&normalized); err != nil {
		return err
	}
	return compiled.Validate(normalized)
}
