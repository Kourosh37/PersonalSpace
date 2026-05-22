FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/.next ./.next
RUN if [ -d /app/public ]; then cp -r /app/public ./public; else mkdir -p ./public; fi
COPY --from=builder /app/package.json ./package.json
COPY --from=deps /app/node_modules ./node_modules
EXPOSE 3000
CMD ["npm", "run", "start"]