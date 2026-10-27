package shared

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// FormatNumber formatea número a string con 2 decimales y separador de miles con coma.
// Ejemplo: 30999.99 → "30,999.99"
func FormatNumber(value float64) string {
	// Formatear primero con 2 decimales (aplica redondeo)
	full := fmt.Sprintf("%.2f", value)

	// Separar parte entera y decimal
	parts := strings.SplitN(full, ".", 2)
	intStr := parts[0]
	fracStr := parts[1]

	// Agregar comas a la parte entera cada 3 dígitos desde la derecha
	neg := false
	if strings.HasPrefix(intStr, "-") {
		neg = true
		intStr = intStr[1:]
	}

	if len(intStr) > 3 {
		var sb strings.Builder
		first := len(intStr) % 3
		if first > 0 {
			sb.WriteString(intStr[:first])
		}
		for i := first; i < len(intStr); i += 3 {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(intStr[i : i+3])
		}
		intStr = sb.String()
	}

	if neg {
		intStr = "-" + intStr
	}

	return intStr + "." + fracStr
}

// FormatPeriodo formatea el periodo según el tipo.
func FormatPeriodo(dateType, dateStart string) string {
	if dateType == "todos" {
		return "Histórico completo"
	}
	if dateType == "" || dateStart == "" {
		return "Periodo no especificado"
	}

	switch dateType {
	case "anual":
		return fmt.Sprintf("Periodo Anual: %s", dateStart)
	case "mes", "mensual":
		return formatMes(dateStart)
	}
	return fmt.Sprintf("Periodo: %s", dateStart)
}

func formatMes(dateStart string) string {
	monthNames := map[string]string{
		"01": "Enero", "02": "Febrero", "03": "Marzo",
		"04": "Abril", "05": "Mayo", "06": "Junio",
		"07": "Julio", "08": "Agosto", "09": "Septiembre",
		"10": "Octubre", "11": "Noviembre", "12": "Diciembre",
	}

	// dateStart viene como "MM/YYYY"
	for i := 0; i <= len(dateStart)-7; i++ {
		month := dateStart[i : i+2]
		if name, ok := monthNames[month]; ok {
			return fmt.Sprintf("Periodo Mensual: %s de %s", name, dateStart[i+3:])
		}
	}
	return fmt.Sprintf("Periodo Mensual: %s", dateStart)
}

// GetFloatValue obtiene el valor de un puntero float64 o 0 si es nil.
func GetFloatValue(ptr *float64) float64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}

// TriggerGC fuerza garbage collection.
func TriggerGC() {
	runtime.GC()
}

// LogMemoryUsage registra el uso de memoria.
func LogMemoryUsage(step string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("[MEM] %s - Alloc: %d MB, TotalAlloc: %d MB, Sys: %d MB, NumGC: %d",
		step,
		m.Alloc/1024/1024,
		m.TotalAlloc/1024/1024,
		m.Sys/1024/1024,
		m.NumGC,
	)
}

// CopyFile copia un archivo de src a dst.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// CreateZipFromFiles crea un archivo ZIP desde una lista de archivos.
func CreateZipFromFiles(filePaths []string, outputPath string) error {
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creando archivo ZIP: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)

	for _, filePath := range filePaths {
		if _, err := os.Stat(filePath); err != nil {
			zipWriter.Close()
			return fmt.Errorf("archivo no encontrado %s: %v", filePath, err)
		}

		file, err := os.Open(filePath)
		if err != nil {
			zipWriter.Close()
			return fmt.Errorf("error abriendo archivo %s: %v", filePath, err)
		}

		info, err := file.Stat()
		if err != nil {
			file.Close()
			zipWriter.Close()
			return fmt.Errorf("error obteniendo info de %s: %v", filePath, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			file.Close()
			zipWriter.Close()
			return fmt.Errorf("error creando header para %s: %v", filePath, err)
		}

		header.Name = filepath.Base(filePath)
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			file.Close()
			zipWriter.Close()
			return fmt.Errorf("error creando entrada ZIP para %s: %v", filePath, err)
		}

		written, err := io.Copy(writer, file)
		file.Close()
		if err != nil {
			zipWriter.Close()
			return fmt.Errorf("error copiando contenido de %s: %v", filePath, err)
		}

		if written != info.Size() {
			zipWriter.Close()
			return fmt.Errorf("error: bytes escritos (%d) no coinciden con tamaño del archivo (%d) para %s", written, info.Size(), filePath)
		}

		log.Printf("[ZIP] Agregado: %s (%d bytes)", filepath.Base(filePath), written)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("error cerrando ZIP writer: %v", err)
	}

	if err := zipFile.Sync(); err != nil {
		return fmt.Errorf("error sincronizando ZIP: %v", err)
	}

	return nil
}

// ToLatin convierte un string UTF-8 a Windows-1252 (CP1252) para
// que gofpdf renderice correctamente tildes y eñes con fuentes built-in.
// Los caracteres que no existen en CP1252 se reemplazan por '?'.
func ToLatin(s string) string {
	if s == "" {
		return s
	}
	encoder := charmap.Windows1252.NewEncoder()
	result, _, err := transform.String(encoder, s)
	if err != nil {
		return s
	}
	return result
}
