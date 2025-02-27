package gencode

import (
	"bytes"
	"fmt"
	"github.com/fdbug/gencode/tools/filex"
	"github.com/fdbug/gencode/tools/stringx"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const (
	modelModel   = "/model_model.tpl"
	modelSchema  = "/model_schema.tpl"
	modelTyps    = "/model_typs.tpl"
	modelComTyps = "/model_typs_common.tpl"
)

type ModelSchema struct {
	*Dataset
	*ModelConfig
}

type ModelConfig struct {
	*DBConfig
	Tables      []string
	Switch      string
	GoZeroStyle string
}

func NewModelSchema(dataset *Dataset, config *ModelConfig) *ModelSchema {
	model := &ModelSchema{
		Dataset:     dataset,
		ModelConfig: config,
	}
	if model.OutPath == "" {
		model.OutPath = "model"
	}
	if model.GoZeroStyle == "" {
		model.GoZeroStyle = "goZero"
	}
	model.filterTable()

	return model
}

func (m *ModelSchema) Generate() (err error) {
	if m.Switch != switch_file && m.Switch != switch_file_cmd {
		return nil
	}

	schemaPath := m.TemplateFilePath + modelSchema
	typesPath := m.TemplateFilePath + modelTyps
	typesCommonPath := m.TemplateFilePath + modelComTyps
	modelPath := m.TemplateFilePath + modelModel

	t := WithTemplate(schemaPath, typesPath, modelPath, typesCommonPath)

	err = m.genSchema(t)
	if err != nil {
		return err
	}

	err = m.genTypes(t)
	if err != nil {
		return err
	}

	err = m.genModel(t)
	if err != nil {
		return err
	}

	log.Println("model success!")
	exec.Command("gofmt", "-s", "-w", m.OutPath).Run()

	return nil
}

func (m *ModelSchema) filterTable() {
	if len(m.Tables) == 0 {
		return
	}
	ts := make([]*Table, 0)
	for _, table := range m.Dataset.TableSet {
		if inSlice(m.Tables, table.Name) {
			ts = append(ts, table)
		}
	}
	m.Dataset.TableSet = ts
}

func (m *ModelSchema) genSchema(template *template.Template) error {
	buf := new(bytes.Buffer)

	err := template.ExecuteTemplate(buf, strings.TrimLeft(modelSchema, "/"), *m)
	if err != nil {
		return err
	}

	return CreateAndWriteFile(m.OutPath, "schema.go", buf.String())
}

func (m *ModelSchema) genTypes(template *template.Template) error {
	buf := new(bytes.Buffer)
	path := m.OutPath + "/typs.go"

	exists, err := filex.PathExists(path)
	if err != nil || !exists {
		common, err := os.ReadFile(m.TemplateFilePath + modelComTyps)
		if err != nil {
			return err
		}
		return CreateAndWriteFile(m.OutPath, "typs.go", string(append(common, buf.Bytes()...)))
	}
	return nil
}

func (m *ModelSchema) genModel(template *template.Template) error {
	buf := new(bytes.Buffer)

	type tp struct {
		IsCache           bool
		IsDate            bool
		Primary           *Field
		PrimaryFmt        string
		PrimaryFmtV       string
		PrimaryFmtV2      string
		PrimaryFields     string
		PrimaryFieldWhere string
		*Table
	}

	for _, t := range m.Dataset.TableSet {
		tp := &tp{
			IsCache: m.IsCache,
			Table:   t,
		}
		for _, f := range t.Fields {
			if f.IsPrimary {
				tp.Primary = f
				tp.PrimaryFields = fmt.Sprintf("%s , %s %s", tp.PrimaryFields, stringx.StartLower(f.CamelName), f.DataType)
				tp.PrimaryFieldWhere = fmt.Sprintf("%s and %s = ?", tp.PrimaryFieldWhere, f.Name)
				tp.PrimaryFmt = tp.PrimaryFmt + "-%v"
				tp.PrimaryFmtV = tp.PrimaryFmtV + " , data." + f.CamelName
				tp.PrimaryFmtV2 = tp.PrimaryFmtV2 + " , " + stringx.StartLower(f.CamelName)
			}
			if isDateType(f.OriginDataType) {
				f.DataType = "*time.Time"
				tp.IsDate = true
			}
		}
		if tp.Primary == nil {
			continue
		}
		tp.PrimaryFields = strings.TrimPrefix(tp.PrimaryFields, " ,")
		tp.PrimaryFieldWhere = strings.TrimPrefix(tp.PrimaryFieldWhere, " and")
		tp.PrimaryFmt = strings.TrimPrefix(tp.PrimaryFmt, "-")
		tp.PrimaryFmtV = strings.TrimPrefix(tp.PrimaryFmtV, " ,")
		tp.PrimaryFmtV2 = strings.TrimPrefix(tp.PrimaryFmtV2, " ,")

		err := template.ExecuteTemplate(buf, strings.TrimLeft(modelModel, "/"), *tp)
		if err != nil {
			return err
		}

		fileName := getRealNameByStyle(t.CamelName+"Model.go", m.GoZeroStyle)

		err = CreateAndWriteFile(m.OutPath, fileName, buf.String())
		if err != nil {
			return err
		}
		buf.Reset()
	}

	return nil
}
