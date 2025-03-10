# TNEP (Trusted Networks Envoy Plugin)

![GitHub Release](https://img.shields.io/github/v/release/achetronic/tnep)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/achetronic/tnep)
[![Go Report Card](https://goreportcard.com/badge/github.com/achetronic/tnep)](https://goreportcard.com/report/github.com/achetronic/tnep)
![GitHub License](https://img.shields.io/github/license/achetronic/tnep)

![GitHub User's stars](https://img.shields.io/github/stars/achetronic?label=Achetronic%20Stars)
![GitHub followers](https://img.shields.io/github/followers/achetronic?label=Achetronic%20Followers)

> [!IMPORTANT]  
> This is taking advantage of Go 1.24+ and new `go:wasmexport` directive, so using upstream Go.
> Thanks to [proxy-wasm community](https://github.com/proxy-wasm/proxy-wasm-go-sdk/tree/main)

## Description

Envoy WASM plugin to process X-Forwarded-For header. 
With it and a list of configured trusted networks, it gets real client IP, and put it in a custom header

## Motivation

Istio uses Envoy as proxy, and Envoy is quite amazing, but has some limitations. One of them is shown when running
behind a load-balancer that performs dynamic amount of hops before reaching Envoy. 

In that case, it's not possible to use 'xff_num_trusted_hops' parameter present in Envoy, as it needs a fixed amount
of trusted hops.

This plugin comes to fix this issue, being able to check the IPs coming into the `X-Forwarded-For` header, 
deleting those that are inside configured trusted networks, and finally writting client's real IP
in a custom header

That way, you can trust entire network ranges, instead of the number of hops, and get only the IPs that belongs to
foreign networks

## How to deploy

Deploying process for this plugin depends on the target (Istio or pure Envoy). You can find examples for both of them
in [documentation directory](./docs/samples). In fact, these examples are used by us to test, so you can rely on them.


## How to develop

This plugin is developed using Go, but compiled using TinyGo. This is done this way because of a limitation in the 
upstream compiler related to [exported functions when compiling to WebAssembly](https://github.com/tetratelabs/proxy-wasm-go-sdk/blob/main/doc/OVERVIEW.md#tinygo-vs-the-official-go-compiler). 

You don't have to worry about mentioned details, as it's only needed to craft your code and execute the following command:

```console
make build run
```

## How releases are created

Each release of this plugin is completely automated by using [Github Actions' workflows](./github). 
Inside those workflows, we use recipes present at Makefile as much as possible to be completely transparent 
in the process we follow for building this.

Assets belonging to each version can be found attached to the corresponding release. OCI images are not yet published
until the whole process is well tested.


## How to collaborate

We are open to external collaborations for this project. For doing it you must:
- Open an issue explaining the problem
- Fork the repository 
- Make your changes to the code
- Open a PR 

> We are developers and hate bad code. For that reason we ask you the highest quality on each line of code to improve
> this project on each iteration. The code will always be reviewed and tested

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Special mention

This project was done using IDEs from JetBrains. They helped us to develop faster, so we recommend them a lot! 🤓

<img src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains Logo (Main) logo." width="150">