#!/bin/sh

set -e

if [ -z "${OUTPUT_DIR}" ]; then
	OUTPUT_DIR=/tmp/output
fi
if [ -z "${CONFIG_DIR}" ]; then
	CONFIG_DIR=/config
fi
if [ -z "${CONFIG_FILE}" ]; then
	CONFIG_FILE=config.yaml
fi

sleep 3

set +e
rm -r "${OUTPUT_DIR:?}"/*
set -e

dmarc-report-converter -config "${CONFIG_DIR}/${CONFIG_FILE}"
