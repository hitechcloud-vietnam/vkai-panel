# VKAI Panel - giao dien Next.js (thu muc panel/)
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .

# QUAN TRONG: NEXT_PUBLIC_* la bien duoc NHUNG THANG VAO BUNDLE luc `next build`,
# khong phai luc chay. Neu chi cap luc runtime thi bundle da build xong se mang
# chuoi rong va giao dien goi API sai dia chi. Vi vay phai khai bao ARG + ENV
# TRUOC buoc build ben duoi.
#
# Gia tri phai la URL ma TRINH DUYET truy cap duoc.
# Mac dinh de TRONG = same-origin qua reverse proxy cua panel (khuyen nghi).
# TUYET DOI khong dat hostname noi bo kieu http://vkai-core:30110.
ARG NEXT_PUBLIC_API_URL=
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

ENV NEXT_TELEMETRY_DISABLED=1

RUN npm run build

FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
