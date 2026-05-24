package payu

import (
	"strings"
	"testing"
)

func TestVerifyResponseHash_Valid(t *testing.T) {
	params := map[string]string{
		"key":         "test_merchant_key",
		"txnid":       "txn_123",
		"amount":      "100.00",
		"productinfo": "test_product",
		"firstname":   "test_first",
		"email":       "test@example.com",
		"status":      "success",
		"udf1":        "",
		"udf2":        "",
		"udf3":        "",
		"udf4":        "",
		"udf5":        "",
	}
	salt := "test_salt_key"

	computed := computeResponseHash(params, salt)
	params["hash"] = computed

	if !VerifyResponseHash(params, salt) {
		t.Error("expected verification to succeed for valid hash")
	}
}

func TestVerifyResponseHash_TamperedAmount(t *testing.T) {
	params := map[string]string{
		"key":         "test_merchant_key",
		"txnid":       "txn_123",
		"amount":      "100.00",
		"productinfo": "test_product",
		"firstname":   "test_first",
		"email":       "test@example.com",
		"status":      "success",
		"udf1":        "",
		"udf2":        "",
		"udf3":        "",
		"udf4":        "",
		"udf5":        "",
	}
	salt := "test_salt_key"

	params["hash"] = computeResponseHash(params, salt)
	params["amount"] = "200.00"

	if VerifyResponseHash(params, salt) {
		t.Error("expected verification to fail for tampered amount")
	}
}

func TestVerifyResponseHash_EmptyHash(t *testing.T) {
	params := map[string]string{
		"key":    "test_merchant_key",
		"txnid":  "txn_123",
		"status": "success",
		"hash":   "",
	}

	if VerifyResponseHash(params, "salt") {
		t.Error("expected verification to fail for empty hash")
	}
}

func TestVerifyResponseHash_MissingHashField(t *testing.T) {
	params := map[string]string{
		"key":    "test_merchant_key",
		"txnid":  "txn_123",
		"status": "success",
	}

	if VerifyResponseHash(params, "salt") {
		t.Error("expected verification to fail when hash field is missing")
	}
}

func TestVerifyResponseHash_UppercaseHash(t *testing.T) {
	params := map[string]string{
		"key":         "test_merchant_key",
		"txnid":       "txn_123",
		"amount":      "100.00",
		"productinfo": "test_product",
		"firstname":   "test_first",
		"email":       "test@example.com",
		"status":      "success",
		"udf1":        "",
		"udf2":        "",
		"udf3":        "",
		"udf4":        "",
		"udf5":        "",
	}
	salt := "test_salt_key"

	computed := computeResponseHash(params, salt)
	params["hash"] = strings.ToUpper(computed)

	if !VerifyResponseHash(params, salt) {
		t.Error("expected verification to succeed for uppercase hex hash")
	}
}

func TestVerifyResponseHash_WithAdditionalCharges(t *testing.T) {
	params := map[string]string{
		"key":               "test_merchant_key",
		"txnid":             "txn_456",
		"amount":            "500.00",
		"productinfo":       "premium_product",
		"firstname":         "user",
		"email":             "user@test.com",
		"status":            "success",
		"udf1":              "",
		"udf2":              "",
		"udf3":              "",
		"udf4":              "",
		"udf5":              "",
		"additionalCharges": "25.00",
	}
	salt := "my_salt"

	computed := computeResponseHash(params, salt)
	params["hash"] = computed

	if !VerifyResponseHash(params, salt) {
		t.Error("expected verification to succeed with additionalCharges")
	}
}

func TestVerifyResponseHash_WithAdditionalCharges_Tampered(t *testing.T) {
	params := map[string]string{
		"key":               "test_merchant_key",
		"txnid":             "txn_456",
		"amount":            "500.00",
		"productinfo":       "premium_product",
		"firstname":         "user",
		"email":             "user@test.com",
		"status":            "success",
		"udf1":              "",
		"udf2":              "",
		"udf3":              "",
		"udf4":              "",
		"udf5":              "",
		"additionalCharges": "25.00",
	}
	salt := "my_salt"

	params["hash"] = computeResponseHash(params, salt)
	params["additionalCharges"] = "50.00"

	if VerifyResponseHash(params, salt) {
		t.Error("expected verification to fail for tampered additionalCharges")
	}
}

func TestVerifyResponseHash_WrongSalt(t *testing.T) {
	params := map[string]string{
		"key":         "test_merchant_key",
		"txnid":       "txn_123",
		"amount":      "100.00",
		"productinfo": "test_product",
		"firstname":   "test_first",
		"email":       "test@example.com",
		"status":      "success",
		"udf1":        "",
		"udf2":        "",
		"udf3":        "",
		"udf4":        "",
		"udf5":        "",
	}

	params["hash"] = computeResponseHash(params, "correct_salt")

	if VerifyResponseHash(params, "wrong_salt") {
		t.Error("expected verification to fail with wrong salt")
	}
}

func TestVerifyResponseHash_EmptyParams(t *testing.T) {
	params := map[string]string{}

	if VerifyResponseHash(params, "salt") {
		t.Error("expected verification to fail for empty params")
	}
}
