package domain

import (
	"fmt"
	"strings"
)

// FieldError describes one invalid field of an input.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError groups the field errors of an input.
type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (e *ValidationError) add(field, message string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Message: message})
}

// OrNil returns e when it has errors, nil otherwise.
func (e *ValidationError) OrNil() error {
	if e == nil || len(e.Fields) == 0 {
		return nil
	}
	return e
}

// Faixas aceitas no cadastro de transformadores de distribuicao. Sao limites
// de plausibilidade para barrar erros de digitacao (unidade trocada, sinal
// invertido), nao limites regulatorios.
const (
	MaxTransformerKVA       = 10000.0 // trafos de distribuicao: tipicamente 5 a 3000 kVA
	MaxPrimaryVoltageKV     = 138.0
	MinSecondaryVoltageV    = 100.0
	MaxSecondaryVoltageV    = 1000.0
	MaxRegulatoryLossLimit  = 100.0
	MaxTransformerCodeChars = 64
)

// ValidateTransformer checa as regras de negocio do cadastro de um
// transformador (Sprint 3: validacao automatica de dados de entrada).
func ValidateTransformer(t Transformer) error {
	v := &ValidationError{}

	code := strings.TrimSpace(t.Code)
	switch {
	case code == "":
		v.add("code", "codigo e obrigatorio")
	case len(code) > MaxTransformerCodeChars:
		v.add("code", fmt.Sprintf("codigo deve ter no maximo %d caracteres", MaxTransformerCodeChars))
	}

	if t.PowerKVA <= 0 {
		v.add("power_kva", "potencia nominal deve ser maior que zero")
	} else if t.PowerKVA > MaxTransformerKVA {
		v.add("power_kva", fmt.Sprintf("potencia nominal acima de %.0f kVA; confira a unidade", MaxTransformerKVA))
	}

	if t.PrimaryVoltageKV < 0 || t.PrimaryVoltageKV > MaxPrimaryVoltageKV {
		v.add("primary_voltage_kv", fmt.Sprintf("tensao primaria deve estar entre 0 e %.0f kV", MaxPrimaryVoltageKV))
	}

	if t.SecondaryVoltageV != 0 && (t.SecondaryVoltageV < MinSecondaryVoltageV || t.SecondaryVoltageV > MaxSecondaryVoltageV) {
		v.add("secondary_voltage_v", fmt.Sprintf("tensao secundaria deve estar entre %.0f e %.0f V", MinSecondaryVoltageV, MaxSecondaryVoltageV))
	}

	if (t.Lat == nil) != (t.Lng == nil) {
		v.add("lat", "latitude e longitude devem ser informadas juntas")
	}
	if t.Lat != nil && (*t.Lat < -90 || *t.Lat > 90) {
		v.add("lat", "latitude deve estar entre -90 e 90")
	}
	if t.Lng != nil && (*t.Lng < -180 || *t.Lng > 180) {
		v.add("lng", "longitude deve estar entre -180 e 180")
	}

	if t.CoreLossKW != nil && *t.CoreLossKW < 0 {
		v.add("core_loss_kw", "perda no nucleo (P0) nao pode ser negativa")
	}
	if t.WindingLossKW != nil && *t.WindingLossKW < 0 {
		v.add("winding_loss_kw", "perda nos enrolamentos (Pcc) nao pode ser negativa")
	}
	if t.CoreLossKW != nil && t.PowerKVA > 0 && *t.CoreLossKW > t.PowerKVA {
		v.add("core_loss_kw", "perda no nucleo maior que a potencia nominal; confira a unidade (kW)")
	}
	if t.WindingLossKW != nil && t.PowerKVA > 0 && *t.WindingLossKW > t.PowerKVA {
		v.add("winding_loss_kw", "perda nos enrolamentos maior que a potencia nominal; confira a unidade (kW)")
	}

	if t.LossLimitPct != nil && (*t.LossLimitPct <= 0 || *t.LossLimitPct > MaxRegulatoryLossLimit) {
		v.add("loss_limit_pct", "limite de perdas deve ser maior que 0 e no maximo 100%")
	}

	return v.OrNil()
}

// ValidUCClass reports whether c is a known consuming unit class.
func ValidUCClass(c UCClass) bool {
	switch c {
	case UCClassResidential, UCClassCommercial, UCClassIndustrial, UCClassRural, UCClassPublicPower:
		return true
	}
	return false
}

// ValidateConsumingUnit checa os campos obrigatorios de uma UC.
func ValidateConsumingUnit(u ConsumingUnit) error {
	v := &ValidationError{}
	if strings.TrimSpace(u.UCCode) == "" {
		v.add("uc_code", "codigo da UC e obrigatorio")
	}
	if u.TransformerID == "" {
		v.add("transformer_id", "transformador e obrigatorio")
	}
	if u.Class != nil && *u.Class != "" && !ValidUCClass(*u.Class) {
		v.add("class", "classe deve ser RESIDENTIAL, COMMERCIAL, INDUSTRIAL, RURAL ou PUBLIC_POWER")
	}
	return v.OrNil()
}
