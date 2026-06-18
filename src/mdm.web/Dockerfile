FROM node:22-alpine AS builder
# Corepack reads the `packageManager` field in package.json (pinned to a
# specific pnpm version) so CI doesn't surprise-break on pnpm@latest releases.
RUN corepack enable
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
ARG VITE_AG_GRID_LICENSE
ENV VITE_AG_GRID_LICENSE=${VITE_AG_GRID_LICENSE}
RUN pnpm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
