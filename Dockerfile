ARG DENO_IMAGE=denoland/deno:alpine-2.3.6

FROM ${DENO_IMAGE}
ENV NODE_ENV=production
EXPOSE 8000
RUN mkdir /app
COPY ./docker-entrypoint.sh /app/entrypoint.sh
RUN chown deno:deno /app
RUN chmod +x /app/entrypoint.sh
USER deno
WORKDIR /app
COPY --chown=deno:deno ["package.json", "package-lock.json*", "tsconfig*.json", "./"]
COPY --chown=deno:deno ["src", "./src"]
ENTRYPOINT [ "/app/entrypoint.sh" ]
RUN deno install
RUN deno cache src/server.ts
CMD [ "deno","run", "--allow-net","--allow-env","--allow-sys","--allow-read","src/server.ts" ]
