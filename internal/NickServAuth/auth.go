package NickServAuth

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type AuthClient struct {
    APIURL    string
    Token     string
    Client    *http.Client
    UserAgent string
}

func NewAuthClient(apiURL, token string) *AuthClient {
    return &AuthClient{
        APIURL: apiURL,
        Token:  token,
        Client: &http.Client{
            Timeout: 10 * time.Second,
        },
        UserAgent: "NickStream/1.0",
    }
}

type AuthRequest struct {
    AccountName string `json:"accountName"`
    Passphrase  string `json:"passphrase"`
}

type AuthResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message,omitempty"`
}

func (a *AuthClient) Authenticate(accountName, passphrase string) (bool, error) {
    reqBody := AuthRequest{
        AccountName: accountName,
        Passphrase:  passphrase,
    }

    jsonData, err := json.Marshal(reqBody)
    if err != nil {
        return false, fmt.Errorf("failed to marshal request: %w", err)
    }

    req, err := http.NewRequest("POST", a.APIURL, bytes.NewBuffer(jsonData))
    if err != nil {
        return false, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Authorization", "Bearer "+a.Token)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", a.UserAgent)

    resp, err := a.Client.Do(req)
    if err != nil {
        return false, fmt.Errorf("request to NickServ API failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return false, fmt.Errorf("NickServ API returned status %d", resp.StatusCode)
    }

    var authResp AuthResponse
    if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
        return false, fmt.Errorf("failed to decode NickServ response: %w", err)
    }

    if !authResp.Success && authResp.Message != "" {
        return false, fmt.Errorf("NickServ authentication failed: %s", authResp.Message)
    }

    return authResp.Success, nil
}
