// Backend/utils/totp.go
package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// GenerateTOTPSecret genera una clave secreta aleatoria codificada en Base32 (16 caracteres).
func GenerateTOTPSecret() (string, error) {
	// 10 bytes de entropía equivalen a 16 caracteres en Base32
	secretBytes := make([]byte, 10)
	_, err := rand.Read(secretBytes)
	if err != nil {
		return "", err
	}
	// Usar codificación Base32 estándar sin padding
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	return secret, nil
}

// GenerateTOTPURI genera una URI de aprovisionamiento estándar para aplicaciones como Google/Microsoft Authenticator.
func GenerateTOTPURI(email, secret string) string {
	// Formato estándar: otpauth://totp/LibreriaLosAltares:email?secret=SECRET&issuer=LibreriaLosAltares
	return fmt.Sprintf("otpauth://totp/LibreriaLosAltares:%s?secret=%s&issuer=LibreriaLosAltares", email, secret)
}

// VerifyTOTP valida si un código de 6 dígitos es correcto para un secreto dado.
// Implementa tolerancia a desvíos de tiempo (revisa ventana actual, anterior y posterior).
func VerifyTOTP(secret string, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}

	// Decodificar el secreto Base32 (tolerar con y sin padding)
	secret = strings.ToUpper(strings.TrimSpace(secret))
	// Si le falta padding, agregarlo o intentar decodificar sin él
	var secretBytes []byte
	var err error
	if len(secret)%8 != 0 {
		secretBytes, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	} else {
		secretBytes, err = base32.StdEncoding.DecodeString(secret)
	}
	if err != nil {
		return false
	}

	// Obtener el intervalo de tiempo actual (pasos de 30 segundos)
	currentTime := time.Now().Unix()
	currentStep := currentTime / 30

	// Ventana de tolerancia: revisar paso anterior (-1), actual (0) y siguiente (+1)
	// Esto cubre retrasos de red y ligeros desfases del reloj del cliente.
	for i := -1; i <= 1; i++ {
		step := currentStep + int64(i)
		if calculateTOTP(secretBytes, step) == code {
			return true
		}
	}

	return false
}

// calculateTOTP realiza la operación criptográfica central del algoritmo TOTP (RFC 6238 / RFC 4226)
func calculateTOTP(secret []byte, step int64) string {
	// Convertir el contador (step) a un arreglo de 8 bytes (Big-Endian)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(step))

	// Calcular HMAC-SHA1
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Truncamiento dinámico (Dynamic Truncation)
	// Tomar los últimos 4 bits (nibble) del hash para definir el offset
	offset := hash[len(hash)-1] & 0x0f

	// Extraer 4 bytes a partir del offset y quedarse con los 31 bits menos significativos
	binaryVal := binary.BigEndian.Uint32(hash[offset : offset+4])
	binaryVal = binaryVal & 0x7fffffff

	// Generar el código de 6 dígitos
	otp := binaryVal % 1000000
	return fmt.Sprintf("%06d", otp)
}
