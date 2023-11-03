# Cli tool to upload and download files on Celestia

## Installation
You need to be running go 1.21, then clone the repo, and make sure to run some kind of node (light or brige) using [celestia-node](https://github.com/celestiaorg/celestia-node/tree/main).
Once your node starts running, make sure you keep your auth token and namespace handy.

## Usage
To submit a file, go to the root of this repo and run:

```
go run main.go -mode=submit -file=<path_to_file> -namespace=<namespace> -auth=<auth token>
```

this will return a height and the hex string of the blob commitment:
```
Succesfully submitted blob to Celestia. Size:  318  Height:  19473  Commitment:  607eae665763ec81e0daa789eb3a44cc4a8859e04282845a47496938b5dbe679
```

With the height and commitment string, you can read from the data blob at the given height and write that data into a file by running:

```
go run main.go -mode=read -file=<file> -namespace=<namespace> -auth=<auth token> -commitment=<hex sting of PFB commitment> -height=<height>
```

This will create the file in the repo's directory with whatever file extension you give it. So if you uploaded a `.jpeg`, make sure you write the data into a file with that file extension, and that you tell others the file extension needed, or have them figure it out :)

## Large files
Files larger than `maxBlobSize` will be chunked and submitted along with a simple manifest file.

```
Successfully read 2553524 bytes from 52259221868_e86daccb7d_6k.jpg
Succesfully submitted blob to Celestia. Size:  1500000  Height:  19471  Commitment:  1a2ffd664905aaac20c9f0bc295ebd1bbe91a3e68abc202d31fb7ecb14463556
Succesfully submitted blob to Celestia. Size:  1053524  Height:  19472  Commitment:  d5375dcd3c2a44884324f7aedcc360888da74532ba2b2facc04e9d48e83a6c12
====================
Succesfully submitted manifest blob to Celestia. Size:  318  Height:  19473  Commitment:  607eae665763ec81e0daa789eb3a44cc4a8859e04282845a47496938b5dbe679
```

To read the entire file back, pass the manifest file's info to the read command, and it will download all the chuncks and reconstruct the file, e.g.

```
go run main.go -mode=read \
-auth=<auth token> \
-file=<file> \
-namespace=cclabs01 \
-commitment=607eae665763ec81e0daa789eb3a44cc4a8859e04282845a47496938b5dbe679 \
-height=19473
```
