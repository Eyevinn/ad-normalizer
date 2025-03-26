ARG NODE_IMAGE=node:23-alpine

FROM ${NODE_IMAGE}
ENV NODE_ENV=production
EXPOSE 8000
RUN mkdir /app
COPY ./docker-entrypoint.sh /app/entrypoint.sh
RUN chown node:node /app
RUN chmod +x /app/entrypoint.sh
USER node
WORKDIR /app
COPY --chown=node:node ["package.json", "package-lock.json*", "tsconfig*.json", "./"]
COPY --chown=node:node ["src", "./src"]
# Delete prepare script to avoid errors from husky
RUN npm pkg delete scripts.prepare \
    && npm ci --omit=dev
ENTRYPOINT [ "/app/entrypoint.sh" ]
CMD [ "npm", "run", "start" ]
