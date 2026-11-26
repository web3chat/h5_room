#!/usr/bin/env bash
# 生成自签名证书，用于 HTTPS 测试（方案 B）。
# 在云服务器上执行（或本机生成后连同 certs/ 一起上传到服务器）。
#
# 用法: bash scripts/gen_selfsigned_cert.sh <公网IP> [额外SAN...]
#   额外 SAN 可选，如: 内网IP、hostname、localhost
#   例: bash scripts/gen_selfsigned_cert.sh 123.45.67.89
#       bash scripts/gen_selfsigned_cert.sh 123.45.67.89 10.0.0.5 localhost
set -euo pipefail

IP="${1:?用法: $0 <公网IP> [额外SAN...]}"
shift || true

mkdir -p certs

SAN="IP:${IP}"
for extra in "$@"; do
  SAN="${SAN},${extra}"
done

# -addext 需要 OpenSSL 1.1.1+ (主流发行版均满足)
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout certs/key.pem \
  -out certs/cert.pem \
  -subj "/CN=${IP}" \
  -addext "subjectAltName=${SAN}"

echo "已生成: certs/cert.pem  certs/key.pem (SAN: ${SAN})"
echo "注意: 证书有效期 365 天; 浏览器需在访问时点过安全警告（见 README）"
