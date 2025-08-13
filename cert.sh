#!/bin/bash

cat > cert.conf <<EOF
[req]
default_bits = 4096
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn

[dn]
CN = <your-ec2-public-dns-or-ip>

[req_ext]
subjectAltName = @alt_names

[alt_names]
DNS.1 = <your-ec2-public-dns>
IP.1 = <your-ec2-public-ip>
EOF

mkdir -p ./certs
openssl req -x509 -newkey rsa:4096 \
  -keyout ./certs/server.key -out ./certs/server.crt \
  -days 365 -nodes \
  -config cert.conf