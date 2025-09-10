/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

# paper-client-go

The Go [PaperCache](https://papercache.io) client. The client supports all commands described in the wire protocol on the homepage.

## Example
```go
client, err := ClientConnect("paper://127.0.0.1:3145")

if err != nil {
  // handle error
}

err := client.Set("hello", "world", 0)

if err != nil {
  // handle error
}

got, err := client.Get("hello")

if err != nil {
  // handle error
}
```
