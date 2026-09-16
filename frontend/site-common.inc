root /usr/share/nginx/html;
index index.html;

gzip on;
gzip_comp_level 5;
gzip_min_length 1024;
gzip_vary on;
gzip_types text/css application/javascript application/json image/svg+xml;

# Vite 产物带内容哈希，可永久缓存
location /assets/ {
    add_header Cache-Control "public, max-age=31536000, immutable";
}

location /activity-icons/ {
    add_header Cache-Control "public, max-age=86400";
}

# index.html 不缓存，保证发版后能拉到新资源
location / {
    add_header Cache-Control "no-cache";
    try_files $uri $uri/ /index.html;
}

location /api/ {
    proxy_pass http://api:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
