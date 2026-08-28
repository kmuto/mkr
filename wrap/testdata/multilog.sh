#!/bin/sh
echo "[INFO] starting batch job"
echo "[INFO] connecting to database"
echo "[WARN] slow query detected: 2.3s" >&2
echo "[INFO] processing 150 records"
echo "[INFO] processed 50/150"
echo "[INFO] processed 100/150"
echo "[WARN] retry: temporary network error" >&2
echo "[INFO] processed 150/150"
echo "[ERROR] failed to send notification email" >&2
echo "[INFO] batch job completed with warnings"
exit 1
