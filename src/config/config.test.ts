import { assertEquals } from "jsr:@std/assert";
import getConfiguration from "./config.ts";
import process from "node:process";

Deno.test("should remove trailing slashes", () => {
  process.env.ENCORE_URL = "http://encore-instance.io/";
  process.env.CALLBACK_LISTENER_URL = "http://callback.com";
  process.env.ASSET_SERVER_URL = "http://assethost.io/";
  process.env.AD_SERVER_URL = "http://adserver.com";
  process.env.REDIS_URL = "http://redis.com";
  process.env.REDIS_CLUSTER = "true";
  process.env.OUTPUT_BUCKET_URL = "s3://ads/";
  process.env.OSC_ACCESS_TOKEN = "token";
  process.env.KEY_FIELD = "Url";
  process.env.KEY_REGEX = "[a-zA-Z]";
  process.env.ENCORE_PROFILE = "test-profile";
  process.env.JIT_PACKAGING = "true";
  process.env.PACKAGING_QUEUE = "ad-packaging";
  process.env.ROOT_URL = "http://eyevinn.ad-normalizer.osaas.io";

  const config = getConfiguration();
  const expectedBucketName = "ads";
  const expectedEncoreUrl = "http://encore-instance.io";
  // Assert
  assertEquals(config.bucket, expectedBucketName);
  assertEquals(config.encoreUrl, expectedEncoreUrl);
  assertEquals(config.keyRegex, "[a-zA-Z]");
  assertEquals(config.keyField, "url");
  assertEquals(config.rediscluster, true);
  assertEquals(config.encoreProfile, "test-profile");
  assertEquals(config.jitPackaging, true);
  assertEquals(config.packagingQueueName, "ad-packaging");
  assertEquals(config.rootUrl, "http://eyevinn.ad-normalizer.osaas.io");
});
