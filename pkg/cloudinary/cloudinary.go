package cloudinary

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// UploadResult holds the response from Cloudinary after a successful upload
type UploadResult struct {
	PublicID     string `json:"public_id"`
	SecureURL    string `json:"secure_url"`
	ResourceType string `json:"resource_type"` // "image", "video", "raw"
	Format       string `json:"format"`
}

// UploadFile uploads a file to Cloudinary under a room-specific folder.
// It uses the unsigned upload via the REST API with authentication signature.
func UploadFile(file multipart.File, filename string, roomID string) (*UploadResult, error) {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("cloudinary credentials not configured")
	}

	folder := fmt.Sprintf("truth-or-dare/rooms/%s", roomID)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Generate signature
	params := map[string]string{
		"folder":    folder,
		"timestamp": timestamp,
	}
	signature := generateSignature(params, apiSecret)

	// Build multipart request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}

	// Add fields
	writer.WriteField("folder", folder)
	writer.WriteField("timestamp", timestamp)
	writer.WriteField("api_key", apiKey)
	writer.WriteField("signature", signature)

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Use "auto" resource type to let Cloudinary detect
	url := fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/auto/upload", cloudName)

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudinary upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cloudinary upload failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result UploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode cloudinary response: %w", err)
	}

	return &result, nil
}

// DeleteResources deletes a list of resources from Cloudinary by their public IDs.
// It groups resources by type and deletes each group.
func DeleteResources(publicIDs []string, resourceTypes []string) error {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		return fmt.Errorf("cloudinary credentials not configured")
	}

	if len(publicIDs) == 0 {
		return nil
	}

	// Group public IDs by resource type
	grouped := make(map[string][]string)
	for i, id := range publicIDs {
		resType := "image" // default
		if i < len(resourceTypes) && resourceTypes[i] != "" {
			resType = resourceTypes[i]
		}
		grouped[resType] = append(grouped[resType], id)
	}

	for resType, ids := range grouped {
		if err := deleteByType(cloudName, apiKey, apiSecret, resType, ids); err != nil {
			fmt.Printf("[Cloudinary] Failed to delete %s resources: %v\n", resType, err)
			// Continue deleting other types even if one fails
		}
	}

	return nil
}

// DeleteFolder deletes a folder from Cloudinary (only works if empty)
func DeleteFolder(roomID string) error {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		return fmt.Errorf("cloudinary credentials not configured")
	}

	folder := fmt.Sprintf("truth-or-dare/rooms/%s", roomID)

	url := fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/folders/%s", cloudName, folder)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(apiKey, apiSecret)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete folder (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func deleteByType(cloudName, apiKey, apiSecret, resourceType string, publicIDs []string) error {
	if len(publicIDs) == 0 {
		return nil
	}

	// Cloudinary DELETE requires public_ids[] as query params, not in the body
	query := url.Values{}
	for _, id := range publicIDs {
		query.Add("public_ids[]", id) // url.Values handles percent-encoding
	}

	fullURL := fmt.Sprintf(
		"https://api.cloudinary.com/v1_1/%s/resources/%s/upload?%s",
		cloudName, resourceType, query.Encode(),
	)

	req, err := http.NewRequest("DELETE", fullURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(apiKey, apiSecret)
	// No Content-Type needed — no body

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[Cloudinary] Deleted %d %s resources\n", len(publicIDs), resourceType)
	return nil
}

// generateSignature creates a Cloudinary API signature from sorted params
func generateSignature(params map[string]string, apiSecret string) string {
	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the string to sign
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	toSign := strings.Join(parts, "&") + apiSecret

	h := sha1.New()
	h.Write([]byte(toSign))
	return fmt.Sprintf("%x", h.Sum(nil))
}
