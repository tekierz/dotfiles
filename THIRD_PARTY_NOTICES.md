# Third-Party Notices

`dotfiles` is distributed under the MIT License, but compiled releases also
contain third-party open-source software under the licenses listed below.
These notices apply to the dependency versions recorded in `go.mod` and
`go.sum`.

The corresponding standard license texts are included in:

- [MIT](LICENSES/MIT.txt)
- [Apache License 2.0](LICENSES/Apache-2.0.txt)
- [pflag BSD 3-Clause](LICENSES/pflag-BSD-3-Clause.txt)
- [x/sys BSD 3-Clause](LICENSES/x-sys-BSD-3-Clause.txt)
- [Go additional patent grant](LICENSES/Go-PATENTS.txt)

This inventory covers packages linked into `cmd/dotfiles`. Development and
release tools are separate programs and are governed by their own licenses.

## Go Runtime and Standard Library

Compiled releases contain portions of the Go runtime and standard library from
the Go toolchain version declared in `go.mod`.

> Copyright 2009 The Go Authors.

The Go runtime and standard library are distributed under the same
[BSD 3-Clause terms](LICENSES/x-sys-BSD-3-Clause.txt) reproduced for
`golang.org/x/sys` below. The Go project's
[additional patent grant](LICENSES/Go-PATENTS.txt) is also included.

## MIT-Licensed Components

| Component | Version | Copyright notice |
|---|---:|---|
| `github.com/aymanbagabas/go-osc52/v2` | v2.0.1 | Copyright (c) 2022 Ayman Bagabas |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | Copyright (c) 2020-2025 Charmbracelet, Inc |
| `github.com/charmbracelet/colorprofile` | v0.2.3-0.20250311203215-f60798e515dc | Copyright (c) 2020-2024 Charmbracelet, Inc |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Copyright (c) 2021-2023 Charmbracelet, Inc |
| `github.com/charmbracelet/x/ansi` | v0.10.1 | Copyright (c) 2023 Charmbracelet, Inc. |
| `github.com/charmbracelet/x/cellbuf` | v0.0.13-0.20250311204145-2c3ea96c31dd | Copyright (c) 2023 Charmbracelet, Inc. |
| `github.com/charmbracelet/x/term` | v0.2.1 | Copyright (c) 2023 Charmbracelet, Inc. |
| `github.com/lucasb-eyer/go-colorful` | v1.2.0 | Copyright (c) 2013 Lucas Beyer |
| `github.com/mattn/go-isatty` | v0.0.20 | Copyright (c) Yasuhiro MATSUMOTO |
| `github.com/mattn/go-runewidth` | v0.0.16 | Copyright (c) 2016 Yasuhiro Matsumoto |
| `github.com/muesli/ansi` | v0.0.0-20230316100256-276c6243b2f6 | Copyright (c) 2021 Christian Muehlhaeuser |
| `github.com/muesli/cancelreader` | v0.2.2 | Copyright (c) 2022 Erik Geiser and Christian Muehlhaeuser |
| `github.com/muesli/termenv` | v0.16.0 | Copyright (c) 2019 Christian Muehlhaeuser |
| `github.com/pelletier/go-toml/v2` | v2.4.3 | Copyright (c) 2021-2023 Thomas Pelletier |
| `github.com/rivo/uniseg` | v0.4.7 | Copyright (c) 2019 Oliver Kuederle |
| `github.com/xo/terminfo` | v0.0.0-20220910002029-abceb7e1c41e | Copyright (c) 2016 Anmol Sethi |

These components are used under the [MIT License](LICENSES/MIT.txt).

## Apache-2.0 Component

`github.com/spf13/cobra` v1.10.2 is licensed under the
[Apache License 2.0](LICENSES/Apache-2.0.txt).

## BSD-3-Clause Components

| Component | Version | Copyright notice | Terms |
|---|---:|---|---|
| `github.com/spf13/pflag` | v1.0.9 | Copyright (c) 2012 Alex Ogier. All rights reserved. Copyright (c) 2012 The Go Authors. All rights reserved. | [License](LICENSES/pflag-BSD-3-Clause.txt) |
| `golang.org/x/sys` | v0.36.0 | Copyright 2009 The Go Authors. | [License](LICENSES/x-sys-BSD-3-Clause.txt) |

The Go project's [additional patent grant](LICENSES/Go-PATENTS.txt) also
accompanies `golang.org/x/sys`.

## go-yaml

`go.yaml.in/yaml/v3` v3.0.4 contains code under both the MIT License and the
Apache License 2.0. The upstream project identifies its files ported from
libyaml as MIT-licensed and the remaining project code as Apache-2.0.

MIT-covered portions:

> Copyright (c) 2006-2010 Kirill Simonov  
> Copyright (c) 2006-2011 Kirill Simonov

Apache-covered portions carry this upstream notice:

> Copyright 2011-2016 Canonical Ltd.
>
> Licensed under the Apache License, Version 2.0 (the "License"); you may not
> use this file except in compliance with the License. You may obtain a copy of
> the License at <https://www.apache.org/licenses/LICENSE-2.0>.
>
> Unless required by applicable law or agreed to in writing, software
> distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
> WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
> License for the specific language governing permissions and limitations
> under the License.

The applicable standard texts are available in
[MIT.txt](LICENSES/MIT.txt) and
[Apache-2.0.txt](LICENSES/Apache-2.0.txt).

## Runtime-Installed Software

`dotfiles` can ask package managers, Git, and npm to install or download
separate third-party programs and configurations. Those works are not
incorporated into the `dotfiles` binary and remain subject to their upstream
licenses. Users should review the displayed plan and the upstream license for
each selected component.
