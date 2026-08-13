# Đưa Sub2API (Docker) ra Internet qua Cloudflare Pro

Giả định: `docker compose` trong `deploy/` đã chạy được, truy cập `http://<IP-VPS>:8080` OK.
Mục tiêu: `https://api.example.com` public, origin ẩn, client IP thật đúng, SSE không bị buffer.

Kiến trúc: `client → Cloudflare → iptables(DOCKER-USER) → Caddy container → sub2api container`

Caddy chạy **trong cùng compose network**. App không publish port ra host nữa.

---

## 1. Compose đã sửa sẵn

`deploy/docker-compose.yml` trong repo này đã có sẵn:

- Khối `ports` của service `sub2api` **đã comment** — app không publish ra host, Caddy gọi qua network nội bộ.
- Service `caddy` (image `caddy:2-alpine`, publish `80`, `443`, `443/udp`), mount `./Caddyfile.cloudflare` và `./certs`.
- Volume `caddy_data`, `caddy_config`, `caddy_logs`.
- Biến `SERVER_TRUSTED_PROXIES` truyền vào service `sub2api`.

Không cần sửa gì thêm ở compose. Kiểm tra sau khi `up`: `ss -lntp | grep -E ':(8080|5432|6379)'` phải **rỗng**, chỉ còn 80/443.

---

## 2. Cert origin: Cloudflare Origin CA

Dashboard → **SSL/TLS → Origin Server → Create Certificate** → hostname `api.example.com` + `*.example.com`, hạn 15 năm.

```bash
cd ~/sub2api/deploy          # đổi cho đúng đường dẫn repo
mkdir -p certs
nano certs/origin.pem        # dán Origin Certificate
nano certs/origin.key        # dán Private Key
chmod 600 certs/origin.key

# CA để bật Authenticated Origin Pulls
curl -fsSL -o certs/cf-origin-pull-ca.pem \
  https://developers.cloudflare.com/ssl/static/authenticated_origin_pull_ca.pem
```

Thêm `deploy/certs/` vào `.gitignore` nếu chưa có.

---

## 3. Caddyfile

`deploy/Caddyfile.cloudflare` đã có sẵn trong repo (bản CDN của `deploy/Caddyfile` baseline). Chỉ cần đổi domain:

```bash
sed -i 's/api\.example\.com/api.your-domain.com/' Caddyfile.cloudflare
```

Khác biệt so với `Caddyfile` gốc — đừng dùng bản gốc sau CDN:

- Khối global: `trusted_proxies static <dải IP Cloudflare>` + `trusted_proxies_strict` + `client_ip_headers CF-Connecting-IP`
- `header_up X-Real-IP {client_ip}` thay vì `{remote_host}` — nếu không, mọi user gộp thành 1 IP egress của Cloudflare
- `reverse_proxy sub2api:8080` thay vì `localhost:8080` (gọi qua compose network)
- TLS: Origin CA cert + `client_auth` (Authenticated Origin Pulls) thay vì cert tự động của Caddy

Không đặt `flush_interval` — Caddy tự flush SSE; giá trị dương gây trễ, `-1` (Caddy 2.6.2) làm request chạy tiếp sau khi client đã ngắt.

Danh sách IP Cloudflare thay đổi theo thời gian — đối chiếu lại `https://www.cloudflare.com/ips-v4` và `ips-v6` mỗi lần chạy script firewall ở bước 5.

Khởi động:

```bash
docker compose up -d
docker compose exec caddy caddy validate --config /etc/caddy/Caddyfile
docker compose logs -f caddy
```

---

## 4. Client IP trong Sub2API

Sai bước này = mọi user gộp thành 1 IP egress của Cloudflare → limiter chống brute-force auth (120 fail/60s rồi block) và API key IP ACL chặn nhầm cả nhà.

### 4a. `trusted_proxies` — đặt trong `deploy/.env`

Biến đã được khai báo sẵn ở `docker-compose.yml` và `.env.example`, mặc định rỗng. Điền vào `.env`:

```dotenv
# Subnet của compose network — chỉ Caddy container nằm trong đó
SERVER_TRUSTED_PROXIES=172.16.0.0/12
```

Lấy subnet chính xác thay cho `172.16.0.0/12` nếu muốn chặt hơn:

```bash
docker network inspect deploy_sub2api-network -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
```

Sửa xong phải `docker compose up -d sub2api` để container nhận env mới (`restart` không nạp lại env).

### 4b. Tắt chế độ tương thích cũ — làm trong Admin UI

`security.trust_forwarded_ip_for_api_key_acl` mặc định **true** và được lưu trong DB, sửa file config không thắng được giá trị DB.

Admin UI → **Settings → Security** → tắt *"Trust forwarded IP for API Key ACL"* (`api_key_acl_trust_forwarded_ip`). Có hiệu lực ngay, không cần restart.

Sau khi tắt, chỉ `SERVER_TRUSTED_PROXIES` quyết định — Caddy đã ghi đè `X-Forwarded-For` bằng IP thật, app chỉ tin Caddy.

> Đổi switch này làm đổi IP fingerprint của session đang mở → user đang đăng nhập có thể bị logout một lần. Làm lúc vắng.

---

## 5. Firewall — ufw KHÔNG chặn được port của Docker

Docker chèn rule DNAT vào chain `DOCKER-USER`, chạy **trước** chain của ufw. `ufw deny 443` sẽ không có tác dụng gì với port đã publish. Phải viết rule vào `DOCKER-USER`.

```bash
sudo tee /usr/local/bin/cf-docker-firewall.sh >/dev/null <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

v4=$(curl -fsSL https://www.cloudflare.com/ips-v4)
v6=$(curl -fsSL https://www.cloudflare.com/ips-v6)
[ -n "$v4" ] && [ -n "$v6" ] || { echo "khong tai duoc danh sach IP Cloudflare"; exit 1; }

iptables -F DOCKER-USER
iptables -A DOCKER-USER -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
for ip in $v4; do
  iptables -A DOCKER-USER -s "$ip" -p tcp -m multiport --dports 80,443 -j RETURN
done
iptables -A DOCKER-USER -p tcp -m multiport --dports 80,443 -j DROP
iptables -A DOCKER-USER -j RETURN

ip6tables -F DOCKER-USER
ip6tables -A DOCKER-USER -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
for ip in $v6; do
  ip6tables -A DOCKER-USER -s "$ip" -p tcp -m multiport --dports 80,443 -j RETURN
done
ip6tables -A DOCKER-USER -p tcp -m multiport --dports 80,443 -j DROP
ip6tables -A DOCKER-USER -j RETURN

netfilter-persistent save
EOF
sudo chmod +x /usr/local/bin/cf-docker-firewall.sh
sudo apt install -y iptables-persistent
sudo /usr/local/bin/cf-docker-firewall.sh
```

> **Cảnh báo:** script `iptables -F DOCKER-USER` xoá sạch rule hiện có trong chain đó. Xem trước bằng `sudo iptables -L DOCKER-USER -n --line-numbers`. Rule chỉ đụng tới TCP 80/443 nên SSH cổng 22 không bị ảnh hưởng, nhưng vẫn nên giữ phiên SSH hiện tại tới khi mở được phiên thứ hai để xác nhận.

Ufw vẫn nên bật cho traffic tới chính host (SSH):

```bash
sudo ufw default deny incoming
sudo ufw allow 22/tcp
sudo ufw enable
```

Cron cập nhật hàng tháng: `0 4 1 * * root /usr/local/bin/cf-docker-firewall.sh`

Nếu VPS có firewall của provider (AWS SG, Vultr, Hetzner Cloud Firewall) thì làm thêm ở đó — lớp đó chặn trước khi gói tin tới máy, tin cậy hơn.

---

## 6. Cấu hình Cloudflare Dashboard

### DNS
- `A  api  <IP VPS>` — **Proxied (đám mây cam)**.
- Xoá mọi record cũ còn trỏ IP thật ở dạng DNS-only (`ftp`, `mail`, `direct`, `cpanel`...) — cách lộ origin phổ biến nhất.

### SSL/TLS
- Mode: **Full (strict)**
- Edge Certificates → Minimum TLS **1.2**, TLS 1.3 **On**, **Always Use HTTPS** On
- **Authenticated Origin Pulls: On** (khớp `client_auth` trong Caddyfile)
- HSTS: bật **sau khi** chạy ổn định vài ngày

### Network
- **WebSockets: On**
- **HTTP/2, HTTP/3: On**

### Speed / Optimization — TẮT hết cho host API
- **Rocket Loader: Off** (chèn JS vào response)
- **Email Obfuscation: Off** (viết lại nội dung)
- **Auto Minify: Off**

### Bot & WAF — chỗ hay chặn nhầm API client nhất
Pro có Super Bot Fight Mode. `curl`, SDK Python, Claude Code, Cherry Studio... đều bị xếp "definitely automated" → block hoặc challenge, client nhận 403 khó hiểu.

- **Security → Bots → Super Bot Fight Mode**: "Definitely automated" = **Allow**, hoặc giữ block rồi dựa vào skip rule dưới.
- **WAF → Custom rules**, tạo rule **Skip**, đẩy lên trên cùng:
  - Expression: `(starts_with(http.request.uri.path, "/v1/")) or (starts_with(http.request.uri.path, "/api/")) or (http.request.uri.path eq "/health")`
  - Action **Skip** → tick *All remaining custom rules*, *Managed rules*, *Super Bot Fight Mode*, *Rate limiting rules*
- Managed Ruleset (OWASP) hay false-positive với prompt chứa SQL/JS/shell. User báo 403 ngẫu nhiên → **Security Events**, lọc Ray ID, tắt đúng rule ID đó.

### Rate Limiting Rules (Pro có)
Chỉ đặt trên đường login, đừng đặt lên `/v1/*`:
- Path chứa `/api/auth/login` → 10 requests / 60s / IP → Block 10 phút

### Cache Rules
- Rule 1: path bắt đầu `/v1/` hoặc `/api/` → **Bypass cache**
- Rule 2 (tuỳ chọn): path bắt đầu `/assets/` → Cache Everything, Edge TTL 1 năm (asset Vite có hash trong tên)

### Compression Rules
- `http.response.content_type contains "text/event-stream"` → **Compression: off**. Bảo hiểm thêm cho SSE.

---

## 7. Giới hạn Cloudflare không vượt được ở gói Pro

| Giới hạn | Giá trị | Ảnh hưởng |
|---|---|---|
| Chờ byte đầu từ origin | **100s** → lỗi **524** | Request non-stream dài (image gen, model chậm) bị đứt. Dùng `stream: true`; giữ `GATEWAY_IMAGE_STREAM_KEEPALIVE_INTERVAL=10` để có byte sớm. Pro không chỉnh được. |
| Max upload body | **100 MB** | `max_request_body_size: 268435456` (256MB) không bao giờ đạt tới qua CF. Muốn hơn phải lên Business (200MB) / Enterprise. |
| Idle kết nối stream | Không giới hạn khi còn byte chảy | Nên keepalive SSE là bắt buộc. |

---

## 8. Kiểm tra sau khi lên

```bash
# 1. Đi qua Cloudflare
curl -sSI https://api.example.com/health | grep -iE 'cf-ray|server|http/'

# 2. Vào thẳng IP phải BỊ CHẶN
curl -sS --max-time 5 http://<IP-VPS>:8080/health ; echo "exit=$?"   # phải fail (không publish port)
curl -sS --max-time 5 -k https://<IP-VPS>/health   ; echo "exit=$?"  # phải fail (firewall + Origin Pull)

# 3. IP thật tới được app chưa — phải ra IP nhà bạn, KHÔNG phải 172.68.x.x / 162.158.x.x
docker compose exec caddy sh -c 'tail -n 5 /var/log/caddy/sub2api.log' | jq '.request.client_ip'

# 4. SSE không buffer: token nhỏ giọt, không dồn 1 cục cuối
curl -N -sS https://api.example.com/v1/chat/completions \
  -H "Authorization: Bearer <api-key>" -H 'Content-Type: application/json' \
  -d '{"model":"<model>","stream":true,"messages":[{"role":"user","content":"đếm từ 1 tới 30"}]}'
```

Cuối cùng vào Admin UI → log request, cột IP phải là IP thật của client. Nếu thấy toàn IP Cloudflare → xem lại bước 4.

---

## 9. Việc còn lại trên app

Trong `deploy/.env`:

```dotenv
# Để trống = sinh ngẫu nhiên mỗi lần restart → mất sạch session, hỏng 2FA
JWT_SECRET=<openssl rand -hex 32>
TOTP_ENCRYPTION_KEY=<openssl rand -hex 32>
POSTGRES_PASSWORD=<mật khẩu mạnh>
REDIS_PASSWORD=<mật khẩu mạnh>
TZ=Asia/Ho_Chi_Minh
```

Trong Admin UI hoặc config:
- `server.frontend_url: "https://api.example.com"` — link reset password trong email
- `cors.allowed_origins` — thêm domain nếu gọi từ web app khác
- `webauthn.rp_id: "api.example.com"`, `rp_origins: ["https://api.example.com"]` nếu bật passkey

Backup định kỳ:

```bash
docker compose exec -T postgres pg_dump -U sub2api sub2api | gzip > backup-$(date +%F).sql.gz
```

---

## 10. Cập nhật app

```bash
cd ~/sub2api/deploy
docker compose pull sub2api
docker compose up -d sub2api
docker compose logs -f sub2api
```

Caddy không cần đụng tới. Đổi Caddyfile thì `docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile` — reload nóng, không rớt kết nối.
