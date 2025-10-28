#!/bin/bash
#
# custom-lcms.sh
# Copyright (C) 2025 Kovid Goyal <kovid at kovidgoyal.net>
#
# Distributed under terms of the MIT license.
#
cd lcms
dist=`pwd`/dist
if [[ ! -d "$dist" ]]; then
    ./configure --prefix="$dist" || exit 1
fi
make -j8 && make install && cd - && \
    LD_LIBRARY_PATH=`pwd`/lcms/dist go test -tags lcms2cgo -run Develop -v ./prism
