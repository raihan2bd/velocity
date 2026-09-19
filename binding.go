package velocity

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// BindJSON strictly decodes exactly one JSON document and validates the result.
// Unknown fields are rejected to prevent silent client/server contract drift.
func (c *Context) BindJSON(target any) error {
	if target == nil {
		return NewHTTPError(http.StatusBadRequest, "binding target is required")
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return normalizeBindError(err)
	}
	// Decode one more value only to enforce the single-document contract. A
	// concrete zero-size target avoids allocating an interface value here.
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return NewHTTPError(http.StatusBadRequest, "request body must contain one JSON value")
		}
		return normalizeBindError(err)
	}
	value := reflect.ValueOf(target)
	for value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		return Validate(target)
	}
	return nil
}

// BindQuery binds URL query values using query tags, then form and json tags.
func (c *Context) BindQuery(target any) error {
	return bindValues(target, c.Request.URL.Query(), "query")
}

// BindForm parses application/x-www-form-urlencoded data and validates it.
func (c *Context) BindForm(target any) error {
	if err := c.Request.ParseForm(); err != nil {
		return normalizeBindError(err)
	}
	return bindValues(target, c.Request.PostForm, "form")
}

// BindMultipart parses multipart form fields (not files) and validates them.
// Use FormFile or MultipartFiles to obtain uploaded files.
func (c *Context) BindMultipart(target any, maxMemory int64) error {
	if maxMemory <= 0 {
		return NewHTTPError(http.StatusBadRequest, "multipart memory limit must be positive")
	}
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		return normalizeBindError(err)
	}
	return bindValues(target, c.Request.MultipartForm.Value, "form")
}

// Bind chooses JSON, URL-encoded form, or multipart binding from Content-Type.
func (c *Context) Bind(target any) error {
	switch c.ContentType() {
	case "application/json", "text/json":
		return c.BindJSON(target)
	case "application/x-www-form-urlencoded":
		return c.BindForm(target)
	case "multipart/form-data":
		return c.BindMultipart(target, 32<<20)
	default:
		return ErrUnsupportedMedia
	}
}

// FormFile obtains one multipart upload. Install MaxBodyBytes before this route
// to set a total request limit; maxMemory governs only memory-vs-disk buffering.
func (c *Context) FormFile(name string, maxMemory int64) (*multipart.FileHeader, error) {
	if maxMemory <= 0 {
		return nil, NewHTTPError(http.StatusBadRequest, "multipart memory limit must be positive")
	}
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		return nil, normalizeBindError(err)
	}
	files := c.Request.MultipartForm.File[name]
	if len(files) == 0 {
		return nil, NewHTTPError(http.StatusBadRequest, "required file is missing")
	}
	return files[0], nil
}

// MultipartFiles returns every upload for a field.
func (c *Context) MultipartFiles(name string, maxMemory int64) ([]*multipart.FileHeader, error) {
	if maxMemory <= 0 {
		return nil, NewHTTPError(http.StatusBadRequest, "multipart memory limit must be positive")
	}
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		return nil, normalizeBindError(err)
	}
	files := c.Request.MultipartForm.File[name]
	if len(files) == 0 {
		return nil, NewHTTPError(http.StatusBadRequest, "required file is missing")
	}
	return append([]*multipart.FileHeader(nil), files...), nil
}

// SaveUploadedFile copies an uploaded file using exclusive creation and 0600
// permissions. filename must be a simple local filename. The destination
// directory must already exist and should not be publicly executable.
func SaveUploadedFile(file *multipart.FileHeader, directory, filename string) (string, error) {
	if file == nil {
		return "", NewHTTPError(http.StatusBadRequest, "file is required")
	}
	if filename == "" {
		filename = file.Filename
	}
	filename = sanitizeFilename(filename)
	if filename == "" || filename == "." || !isSimpleFilename(filename) {
		return "", NewHTTPError(http.StatusBadRequest, "invalid upload filename")
	}
	destination := filepath.Join(directory, filename)
	source, err := file.Open()
	if err != nil {
		return "", WrapHTTPError(http.StatusBadRequest, "cannot read uploaded file", err)
	}
	defer source.Close()
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", WrapHTTPError(http.StatusConflict, "upload destination already exists or is unavailable", err)
	}
	success := false
	defer func() {
		if !success {
			_ = destinationFile.Close()
			_ = os.Remove(destination)
		}
	}()
	if _, err = io.Copy(destinationFile, source); err != nil {
		return "", WrapHTTPError(http.StatusInternalServerError, "cannot save uploaded file", err)
	}
	if err = destinationFile.Close(); err != nil {
		return "", WrapHTTPError(http.StatusInternalServerError, "cannot save uploaded file", err)
	}
	success = true
	return destination, nil
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return filepath.Base(name)
}

func isSimpleFilename(name string) bool {
	return filepath.IsLocal(name) && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\")
}

func normalizeBindError(err error) error {
	if err == nil {
		return nil
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return ErrPayloadTooLarge
	}
	return WrapHTTPError(http.StatusBadRequest, "invalid request body", err)
}

func bindValues(target any, values url.Values, tagName string) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return NewHTTPError(http.StatusBadRequest, "binding target must be a non-nil pointer to a struct")
	}
	if err := bindStruct(value.Elem(), values, tagName); err != nil {
		return err
	}
	return Validate(target)
}

func bindStruct(value reflect.Value, values url.Values, tagName string) error {
	typeInfo := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := typeInfo.Field(index)
		if field.PkgPath != "" {
			continue
		}
		fieldValue := value.Field(index)
		if field.Anonymous && dereferencedKind(fieldValue) == reflect.Struct && field.Tag.Get(tagName) == "" {
			if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
				continue
			}
			if err := bindStruct(indirectValue(fieldValue), values, tagName); err != nil {
				return err
			}
			continue
		}
		name := bindingName(field, tagName)
		if name == "" {
			continue
		}
		raw, ok := values[name]
		if !ok {
			continue
		}
		if err := setField(fieldValue, raw); err != nil {
			return WrapHTTPError(http.StatusBadRequest, "invalid value for "+name, err)
		}
	}
	return nil
}

func bindingName(field reflect.StructField, preferred string) string {
	for _, tagName := range []string{preferred, "form", "json"} {
		if tag, ok := field.Tag.Lookup(tagName); ok {
			name := strings.Split(tag, ",")[0]
			if name == "-" {
				return ""
			}
			if name != "" {
				return name
			}
		}
	}
	return field.Name
}

func setField(value reflect.Value, raw []string) error {
	if !value.CanSet() {
		return nil
	}
	if value.Kind() == reflect.Pointer {
		if len(raw) == 0 {
			return nil
		}
		value.Set(reflect.New(value.Type().Elem()))
		return setField(value.Elem(), raw)
	}
	if value.Kind() == reflect.Slice {
		result := reflect.MakeSlice(value.Type(), len(raw), len(raw))
		for index, item := range raw {
			if err := setSingle(result.Index(index), item); err != nil {
				return err
			}
		}
		value.Set(result)
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	return setSingle(value, raw[0])
}

func setSingle(value reflect.Value, raw string) error {
	if value.CanAddr() {
		if unmarshaler, ok := value.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return unmarshaler.UnmarshalText([]byte(raw))
		}
	}
	if value.Type() == reflect.TypeFor[time.Duration]() {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		value.SetInt(int64(parsed))
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		value.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		value.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported field type %s", value.Type())
	}
	return nil
}

func indirectValue(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}
func dereferencedKind(value reflect.Value) reflect.Kind {
	for value.Kind() == reflect.Pointer {
		return value.Type().Elem().Kind()
	}
	return value.Kind()
}

// Validate checks exported fields' comma-separated validate tags. Supported
// rules: required, omitempty, min=N, max=N, len=N, email, url, uuid, alpha,
// alphanum, numeric, oneof=a b, and dive for slices or arrays.
func Validate(target any) error {
	if target == nil {
		return NewHTTPError(http.StatusBadRequest, "validation target is required")
	}
	value := reflect.ValueOf(target)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ValidationErrors{{Field: "", Tag: "required"}}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return NewHTTPError(http.StatusBadRequest, "validation target must be a struct or pointer to struct")
	}
	if !typeNeedsValidation(value.Type()) {
		return nil
	}
	errors := validateStruct(value, "")
	if len(errors) > 0 {
		return errors
	}
	return nil
}

var validationTypeCache sync.Map // map[reflect.Type]bool

// typeNeedsValidation prevents ordinary binding targets with no validate tags
// from paying the reflective field walk. It preserves nested validation: a tag
// on an exported child struct still marks its enclosing type as validated.
func typeNeedsValidation(typ reflect.Type) bool {
	if cached, ok := validationTypeCache.Load(typ); ok {
		return cached.(bool)
	}
	needsValidation := scanValidationTags(typ, make(map[reflect.Type]bool))
	actual, _ := validationTypeCache.LoadOrStore(typ, needsValidation)
	return actual.(bool)
}

func scanValidationTags(typ reflect.Type, visiting map[reflect.Type]bool) bool {
	if typ.Kind() != reflect.Struct || visiting[typ] {
		return false
	}
	visiting[typ] = true
	defer delete(visiting, typ)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		if rules := field.Tag.Get("validate"); rules != "" && rules != "-" {
			return true
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType != reflect.TypeFor[time.Time]() && scanValidationTags(fieldType, visiting) {
			return true
		}
	}
	return false
}

func validateStruct(value reflect.Value, prefix string) ValidationErrors {
	var failures ValidationErrors
	typeInfo := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := typeInfo.Field(index)
		if field.PkgPath != "" {
			continue
		}
		fieldValue := value.Field(index)
		name := field.Name
		if jsonName := strings.Split(field.Tag.Get("json"), ",")[0]; jsonName != "" && jsonName != "-" {
			name = jsonName
		}
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}
		if field.Anonymous && field.Tag.Get("validate") == "" && dereferencedKind(fieldValue) == reflect.Struct {
			if fieldValue.Kind() != reflect.Pointer || !fieldValue.IsNil() {
				failures = append(failures, validateStruct(indirectValue(fieldValue), prefix)...)
			}
			continue
		}
		failures = append(failures, validateField(fieldValue, fullName, field.Tag.Get("validate"))...)
		base := indirectForValidation(fieldValue)
		if base.IsValid() && base.Kind() == reflect.Struct && base.Type() != reflect.TypeFor[time.Time]() {
			failures = append(failures, validateStruct(base, fullName)...)
		}
	}
	return failures
}

func validateField(value reflect.Value, name, rules string) ValidationErrors {
	if rules == "" || rules == "-" {
		return nil
	}
	parts := strings.Split(rules, ",")
	for _, rule := range parts {
		if rule == "omitempty" && isEmpty(value) {
			return nil
		}
	}
	var failures ValidationErrors
	for ruleIndex, rule := range parts {
		if rule == "" || rule == "omitempty" {
			continue
		}
		if rule == "dive" {
			base := indirectForValidation(value)
			if base.IsValid() && (base.Kind() == reflect.Slice || base.Kind() == reflect.Array) {
				elementRules := strings.Join(parts[ruleIndex+1:], ",")
				for index := 0; index < base.Len(); index++ {
					failures = append(failures, validateField(base.Index(index), name+"["+strconv.Itoa(index)+"]", elementRules)...)
				}
			}
			break
		}
		if !validRule(value, rule) {
			failures = append(failures, ValidationError{Field: name, Tag: rule})
		}
	}
	return failures
}

func validRule(value reflect.Value, rule string) bool {
	base := indirectForValidation(value)
	if rule == "required" {
		return !isEmpty(value)
	}
	if !base.IsValid() {
		return false
	}
	key, argument, _ := strings.Cut(rule, "=")
	switch key {
	case "min", "max", "len":
		limit, err := strconv.ParseFloat(argument, 64)
		if err != nil {
			return false
		}
		measure, ok := lengthOrNumber(base)
		if !ok {
			return false
		}
		if key == "min" {
			return measure >= limit
		}
		if key == "max" {
			return measure <= limit
		}
		return measure == limit
	case "email":
		text, ok := asString(base)
		if !ok {
			return false
		}
		parsed, err := mail.ParseAddress(text)
		return err == nil && parsed.Address == text
	case "url":
		text, ok := asString(base)
		if !ok {
			return false
		}
		parsed, err := url.ParseRequestURI(text)
		return err == nil && parsed.Scheme != "" && parsed.Host != ""
	case "uuid":
		text, ok := asString(base)
		return ok && validUUID(text)
	case "alpha":
		text, ok := asString(base)
		if !ok || text == "" {
			return false
		}
		for _, char := range text {
			if !unicode.IsLetter(char) {
				return false
			}
		}
		return true
	case "alphanum":
		text, ok := asString(base)
		if !ok || text == "" {
			return false
		}
		for _, char := range text {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
				return false
			}
		}
		return true
	case "numeric":
		text, ok := asString(base)
		if !ok || text == "" {
			return false
		}
		_, err := strconv.ParseFloat(text, 64)
		return err == nil
	case "oneof":
		text, ok := asString(base)
		if !ok {
			return false
		}
		for _, option := range strings.Fields(argument) {
			if text == option {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func indirectForValidation(value reflect.Value) reflect.Value {
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func isEmpty(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	return value.IsZero()
}

func lengthOrNumber(value reflect.Value) (float64, bool) {
	switch value.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		return float64(value.Len()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		return value.Float(), true
	}
	return 0, false
}

func asString(value reflect.Value) (string, bool) {
	if value.Kind() == reflect.String {
		return value.String(), true
	}
	return "", false
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
