# Third-party notices

## oh-my-pi — Devin subscription protocol adapter

The Devin implementation in `backend/internal/pkg/devin/` adapts the authentication,
model discovery, protobuf field definitions and Connect streaming logic from
[oh-my-pi](https://github.com/can1357/oh-my-pi), commit
`4049c62fbb0a7f5068954d01f11a73112c942a11`:

- `packages/ai/src/providers/devin.ts`
- `packages/catalog/src/discovery/devin.ts`
- `packages/catalog/src/discovery/devin-proto.ts`
- `packages/catalog/src/wire/devin.ts`
- `packages/catalog/src/wire/devin-proto.ts`
- `packages/catalog/src/compat/rules/auth/devin.kdl`

The upstream root and `packages/ai/LICENSE` carry the following MIT notice.
`packages/catalog/LICENSE` carries the same license text with the Can Bölük and
Stencil Labs copyrights below. Runtime images include a copy at
`/app/resources/NOTICE-oh-my-pi.txt`, and release archives include this file.
These notices apply to the adapted upstream material; they do not replace this
project's license.

MIT License

Copyright (c) 2025 Mario Zechner
Copyright (c) 2025-2026 Can Bölük
Copyright (c) 2026 Stencil Labs, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
