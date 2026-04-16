# restapiclient
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/tekkamanendless/restapiclient?label=version&logo=version&sort=semver)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/tekkamanendless/restapiclient)](https://pkg.go.dev/github.com/tekkamanendless/restapiclient)

## Environment Variables
* `RESTAPICLIENT_DUMP_DIRECTORY`; if set, the client will dump the contents of the response to a file in this directory.
   The filename will be a timestamp followed by the method and URL.
   The contents will be written as-is, so you can use this to dump binary data.
   The filename will be sanitized to remove any characters that are not alphanumeric, underscore, or hyphen.
   The file will be created with permissions 0644.
   If the file cannot be created, an error will be logged but the request will continue.

