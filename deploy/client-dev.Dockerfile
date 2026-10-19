# Dev image only: runs the Vite dev server. Sources are bind-mounted by compose.
FROM node:22-alpine
WORKDIR /app
COPY client/package.json client/package-lock.json ./
RUN npm ci
EXPOSE 5173
CMD ["npx", "vite", "--host", "0.0.0.0", "--port", "5173"]
