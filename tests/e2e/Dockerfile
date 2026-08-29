FROM mcr.microsoft.com/playwright:v1.55.0-noble

WORKDIR /work
COPY package.json package-lock.json ./
RUN npm ci --omit=optional --no-audit --no-fund

COPY playwright.config.js upload.spec.js ./

CMD ["npx", "playwright", "test"]
