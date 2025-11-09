FROM node:20-alpine AS build
WORKDIR /workspace
COPY clients/angular-auth ./clients/angular-auth
WORKDIR /workspace/ai/frontend
COPY ai/frontend/package.json ./package.json
COPY ai/frontend/yarn.lock ./yarn.lock
RUN corepack enable \
 && if [ -f yarn.lock ]; then yarn install --immutable; else yarn install; fi \
 && ln -s /workspace/ai/frontend/node_modules /workspace/node_modules
COPY ai/frontend/ .
RUN yarn build

FROM nginx:1.27-alpine
# Angular application builder outputs browser assets under dist/<app>/browser
COPY --from=build /workspace/ai/frontend/dist/aiapp/browser /usr/share/nginx/html
COPY ai/frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx","-g","daemon off;"]
