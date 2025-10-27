#!/bin/bash
#
# custom-lcms.sh
# Copyright (C) 2025 Kovid Goyal <kovid at kovidgoyal.net>
#
# Distributed under terms of the MIT license.
#
cd /t/lcms2-* && make -j8 && make install && cd - && \
    LD_LIBRARY_PATH=/t/lc/lib go test -tags lcms2cgo -run Develop -v ./prism
