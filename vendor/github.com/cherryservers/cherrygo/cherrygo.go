package cherrygo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
)

const (
	apiURL             = "https://api.cherryservers.com/v1/"
	cherryAuthTokenVar = "CHERRY_AUTH_TOKEN"
	mediaType          = "application/json"
	userAgent          = "cherry-agent-go"
)

// Client returns struct for client
type Client struct {
	client *http.Client
	debug  bool

	BaseURL *url.URL

	UserAgent string
	AuthToken string

	Teams       GetTeams
	Plans       GetPlans
	Images      GetImages
	Project     GetProject
	Projects    GetProjects
	SSHKeys     GetSSHKeys
	SSHKey      GetSSHKey
	Servers     GetServers
	Server      GetServer
	IPAddresses GetIPS
	IPAddress   GetIP
}

// Response is the http response from api calls
type Response struct {
	*http.Response
}

// MakeRequest makes request to API
func (c *Client) MakeRequest(method, path string, body, v interface{}) (*Response, error) {

	url, _ := url.Parse(path)

	u := c.BaseURL.ResolveReference(url)
	fmt.Printf("\nAPI Endpoint: %v\n", u)
	//fmt.Printf("\nBODY: %v\n", body)

	buf := new(bytes.Buffer)
	if body != nil {
		coder := json.NewEncoder(buf)
		err := coder.Encode(body)
		if err != nil {
			log.Printf("Error while encoding body: %v -> %v", err, err.Error())
			return nil, err
		}
	}

	//fmt.Printf("\nBODY: %v\n", buf)

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Close = true

	bearer := "Bearer " + c.AuthToken
	req.Header.Add("Authorization", bearer)

	// New request cia baigiasi

	// cia darom jau realu kvietima i api
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// make debug = 1 to output raw JSON
	debug := 0
	if debug == 1 {

		// Debug output of Json body
		fmt.Printf("REQUEST: %v", resp)
		bodyBytes, err2 := ioutil.ReadAll(resp.Body)
		if err2 != nil {
			log.Fatal("FATAL: ", err2)
		}
		fmt.Printf("%+v", string(bodyBytes))
		fmt.Println("----------")
	}
	defer resp.Body.Close()

	// inicializuojam responsa reikiamo tipo grazinimui
	response := Response{Response: resp}

	if sc := response.StatusCode; sc >= 299 {

		type ErrorResponse struct {
			Response *http.Response
			Code     int    `json:"code"`
			Message  string `json:"message"`
		}

		var errorResponse ErrorResponse

		bod, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Error while reading error body: %v", err)
		}
		err = json.Unmarshal(bod, &errorResponse)
		if err != nil {
			fmt.Printf("Error while unmarshal error body: %v", err)
		}
		// jei reikia viso, tai printinti response.Response
		err = fmt.Errorf("Error response from API: %v (error code: %v)", errorResponse.Message, errorResponse.Code)

		return &response, err
	}

	// errR := &ErrorResponse{Response: resp}
	// data, err := ioutil.ReadAll(resp.Body)
	// if err == nil && len(data) > 0 {
	// 	json.Unmarshal(data, errR)
	// }

	// fmt.Printf("BBB: %v", errR.Response)

	// Handling delete requests which EOF is not an error
	if method == "DELETE" && response.StatusCode == 204 {
		return &response, err
	}

	if v != nil {
		// if v implements the io.Writer interface, return the raw response
		if w, ok := v.(io.Writer); ok {
			io.Copy(w, resp.Body)
		} else {

			decoder := json.NewDecoder(resp.Body)
			err := decoder.Decode(&v)
			if err != nil {
				log.Printf("Error while decoding body: %v -> %v", err, err.Error())
				return &response, err
			}
		}
	}

	return &response, nil
}

// NewClient initialization
func NewClient() (*Client, error) {

	httpClient := &http.Client{}

	authToken := os.Getenv(cherryAuthTokenVar)
	if authToken == "" {
		return nil, fmt.Errorf("You must export %s", cherryAuthTokenVar)
	}

	url, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}

	c := &Client{client: httpClient, AuthToken: authToken, BaseURL: url, UserAgent: userAgent}

	// I teamsClient atiduotu cca turiu apie client'a
	c.Teams = &TeamsClient{client: c}
	c.Plans = &PlansClient{client: c}
	c.Images = &ImagesClient{client: c}
	c.Project = &ProjectClient{client: c}
	c.Projects = &ProjectsClient{client: c}
	c.SSHKeys = &SSHKeysClient{client: c}
	c.SSHKey = &SSHKeyClient{client: c}
	c.Servers = &ServersClient{client: c}
	c.Server = &ServerClient{client: c}
	c.IPAddresses = &IPSClient{client: c}
	c.IPAddress = &IPClient{client: c}

	return c, nil
}

// ErrorResponse fields
type ErrorResponse struct {
	Response    *http.Response
	Errors      []string `json:"errors"`
	SingleError string   `json:"error"`
}

func checkResponseForErrors(r *http.Response) *ErrorResponse {
	if c := r.StatusCode; c >= 200 && c <= 299 {
		return nil
	}

	errR := &ErrorResponse{Response: r}
	data, err := ioutil.ReadAll(r.Body)
	if err == nil && len(data) > 0 {
		json.Unmarshal(data, errR)
	}

	return errR

}
