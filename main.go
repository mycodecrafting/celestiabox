package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/h2non/filetype"
	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"
	"github.com/rollkit/celestia-openrpc/types/share"
)

type DAConfig struct {
	Rpc         string `koanf:"rpc"`
	NamespaceId string `koanf:"namespace-id"`
	AuthToken   string `koanf:"auth-token"`
}

type CelestiaDA struct {
	cfg       DAConfig
	client    *openrpc.Client
	namespace share.Namespace
}

type Chunk struct {
	Blob string `json:"blob"`
	Size int    `json:"size"`
}

type Manifest struct {
	Name     string  `json:"name"`
	MimeType string  `json:"mimeType"`
	Size     int     `json:"size"`
	Chunks   []Chunk `json:"chunks"`
}

func namespaceHexToBlobNamespaceV0(namespaceId string) (share.Namespace, error) {
	nsBytes, err := hex.DecodeString(namespaceId)
	if err != nil {
		return nil, err
	}

	namespace, err := share.NewBlobNamespaceV0(nsBytes)
	if err != nil {
		return nil, err
	}

	return namespace, nil
}

func NewCelestiaDA(cfg DAConfig) (*CelestiaDA, error) {
	daClient, err := openrpc.NewClient(context.Background(), cfg.Rpc, cfg.AuthToken)
	if err != nil {
		return nil, err
	}

	if cfg.NamespaceId == "" {
		return nil, errors.New("namespace id cannot be blank")
	}

	namespace, err := namespaceHexToBlobNamespaceV0(cfg.NamespaceId)
	if err != nil {
		return nil, err
	}

	return &CelestiaDA{
		cfg:       cfg,
		client:    daClient,
		namespace: namespace,
	}, nil
}

func (c *CelestiaDA) Store(ctx context.Context, message []byte) ([]byte, uint64, error) {
	dataBlob, err := blob.NewBlobV0(c.namespace, message)
	if err != nil {
		return nil, 0, err
	}
	commitment, err := blob.CreateCommitment(dataBlob)
	if err != nil {
		return nil, 0, err
	}
	height, err := c.client.Blob.Submit(ctx, []*blob.Blob{dataBlob}, openrpc.DefaultSubmitOptions())
	if err != nil {
		return nil, 0, err
	}
	if height == 0 {
		return nil, 0, errors.New("unexpected response code")
	}

	return commitment, height, nil
}

func (c *CelestiaDA) Read(ctx context.Context, commitment string, height uint64) ([]byte, error) {
	fmt.Println("Requesting data from Celestia", "namespace", c.cfg.NamespaceId, "commitment", commitment, "height", height)

	commitmentBytes, err := hex.DecodeString(commitment)
	if err != nil {
		return nil, fmt.Errorf("Error decoding hex string for commitment:", err)
	}

	blob, err := c.client.Blob.Get(ctx, height, c.namespace, commitmentBytes)
	if err != nil {
		return nil, err
	}

	fmt.Println("Succesfully fetched data from Celestia", "namespace", c.cfg.NamespaceId, "height", height, "commitment", commitment)

	var manifest Manifest
	err = json.Unmarshal(blob.Data, &manifest)
	if err != nil {
		return blob.Data, nil
	}

	fmt.Println("Detected manifest file, fetching data chunks...")

	data := []byte{}
	bytesFetched := 0
	for _, chunk := range manifest.Chunks {
		blobParts := strings.Split(chunk.Blob, "/")
		if len(blobParts) != 3 {
			return nil, fmt.Errorf("Invalid chunk blob format")
		}

		fmt.Println("Requesting data chunk from Celestia", "namespace", blobParts[1], "commitment", blobParts[2], "height", blobParts[0])

		chunkHeight, err := strconv.ParseUint(blobParts[0], 10, 64)
		if err != nil {
			return nil, err
		}

		chunkNamespace, err := namespaceHexToBlobNamespaceV0(blobParts[1])
		if err != nil {
			return nil, err
		}

		commitmentBytes, err := hex.DecodeString(blobParts[2])
		if err != nil {
			return nil, err
		}

		chunkBlob, err := c.client.Blob.Get(ctx, chunkHeight, chunkNamespace, []byte(commitmentBytes))
		if err != nil {
			return nil, err
		}
		data = append(data, chunkBlob.Data...)

		bytesFetched = bytesFetched + chunk.Size

		fmt.Println("Succesfully fetched data from Celestia", "namespace", blobParts[1], "commitment", blobParts[2], "height", blobParts[0], " (", bytesFetched, "/", manifest.Size, " bytes)")
	}
	return data, nil
}

func readFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func writeFile(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0644)
}

func getMimeType(buf []byte) (string, error) {
	kind, err := filetype.Match(buf)
	if err != nil {
		return "application/octet-stream", err
	}
	if kind.MIME.Value == "" {
		return "application/octet-stream", nil
	}
	return kind.MIME.Value, nil
}

func split(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf[:len(buf)])
	}
	return chunks
}

func main() {
	// Define flags
	mode := flag.String("mode", "submit", "Mode of operation: read or write")
	filename := flag.String("file", "", "Path to the file")
	commitment := flag.String("commitment", "", "commitment for the blob")
	namespace := flag.String("namespace", "000008e5f679bf7116cb", "target namespace")
	auth := flag.String("auth", "", "auth token (default is $CELESTIA_NODE_AUTH_TOKEN)")
	height := flag.Uint64("height", 0, "celestia height to fetch a blob from")
	rpc := flag.String("rpc", "http://localhost:26658", "celestia rpc node")
	maxBlobSize := flag.Int("max-blob-size", 1500000, "Max file chunk size in bytes")
	manualMimeType := flag.String("mime-type", "", "Specify file mime type for manifest file. By default, it will attempt to automagically determine the mime type.")

	flag.Parse()

	// Check if filename is provided
	if *auth == "" {
		fmt.Println("Please supply auth token")
		return
	}

	// Check if filename is provided
	if *filename == "" {
		fmt.Println("Please provide a filename using -file=<filename>")
		return
	}
	// Start Celestia DA
	daConfig := DAConfig{
		Rpc:         *rpc,
		NamespaceId: hex.EncodeToString([]byte(*namespace)),
		AuthToken:   *auth,
	}

	celestiaDA, err := NewCelestiaDA(daConfig)
	if err != nil {
		fmt.Println("Error creating Celestia client:", err)
		return
	}

	switch *mode {
	case "submit":
		data, err := readFile(*filename)
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}

		fmt.Printf("Successfully read %d bytes from %s\n", len(data), *filename)

		// Get mime type of the file
		var mimeType string
		if *manualMimeType == "" {
			mimeType, err = getMimeType(data)
			if err != nil {
				fmt.Println("Error getting mime type of the file:", err)
			}
		} else {
			mimeType = *manualMimeType
		}

		// Create a new manifest
		manifest := Manifest{
			Name:     *filename,
			MimeType: mimeType,
			Size:     len(data),
		}

		chunks := split(data, *maxBlobSize)

		for _, chunk := range chunks {
			commitment, height, err := celestiaDA.Store(context.Background(), chunk)
			if err != nil {
				fmt.Println("Error submitting blob to Celestia", err)
				return
			}

			// Append to manifest Chunks
			manifest.Chunks = append(manifest.Chunks, Chunk{
				Blob: fmt.Sprintf("%d/%s/%s", height, daConfig.NamespaceId, hex.EncodeToString(commitment)),
				Size: len(chunk),
			})

			fmt.Println("Succesfully submitted blob to Celestia. Size: ", len(chunk), " Height: ", height, " Commitment: ", hex.EncodeToString(commitment))
		}

		// Submit manifest file
		if len(chunks) > 1 {
			manifestBytes, err := json.Marshal(manifest)
			if err != nil {
				fmt.Println("Error marshalling manifest to byte array:", err)
				return
			}
			manifestCommitment, manifestHeight, err := celestiaDA.Store(context.Background(), manifestBytes)
			if err != nil {
				fmt.Println("Error submitting blob to Celestia:", err)
				return
			}
			fmt.Println(strings.Repeat("=", 20))
			fmt.Println("Succesfully submitted manifest blob to Celestia. Size: ", len(manifestBytes), " Height: ", manifestHeight, " Commitment: ", hex.EncodeToString(manifestCommitment))
		}
	case "read":
		if *commitment == "" {
			fmt.Println("Please provide commitment using -commitment=<commitment>")
			return
		}

		if *height == 0 {
			fmt.Println("Please provide height using -height=<height>")
			return
		}

		data, err := celestiaDA.Read(context.Background(), *commitment, *height)
		if err != nil {
			fmt.Println("Error reading from Celestia:", err)
			return
		}
		err = writeFile(*filename, data)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
		fmt.Println("File written successfully!")
	default:
		fmt.Println("Invalid mode. Please specify either 'read' or 'write'.")
	}
}
