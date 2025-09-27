import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const config = new pulumi.Config();

const lambdaRole = new aws.iam.Role("feednyang-lambda-role", {
  assumeRolePolicy: JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Action: "sts:AssumeRole",
        Effect: "Allow",
        Principal: {
          Service: "lambda.amazonaws.com"
        }
      }
    ]
  })
});

new aws.iam.RolePolicyAttachment("feednyang-lambda-basic-policy", {
  role: lambdaRole.name,
  policyArn: "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
});

const feednyangRssFeedFunc = new aws.lambda.Function("feednyang-rss-feed", {
  code: new pulumi.asset.AssetArchive({
    ".": new pulumi.asset.FileArchive("./lambda/feednyang-rss-feed"),
  }),
  runtime: "provided.al2023",
  handler: "bootstrap",
  role: lambdaRole.arn,
  environment: {
    variables: {
      DISCORD_BOT_TOKEN: config.require("discord-bot-token"),
      DEFAULT_DISCORD_CHANNEL_IDS: config.require("default-discord-channel-ids"),
      MONGODB_URI: config.require("mongodb-uri")
    }
  },
  timeout: 300
});

const eventBridgeRole = new aws.iam.Role("eventbridge-lambda-role", {
  assumeRolePolicy: JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Action: "sts:AssumeRole",
        Effect: "Allow",
        Principal: {
          Service: "events.amazonaws.com"
        }
      }
    ]
  })
});

new aws.iam.RolePolicy("eventbridge-lambda-policy", {
  role: eventBridgeRole.id,
  policy: feednyangRssFeedFunc.arn.apply(arn => JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Effect: "Allow",
        Action: "lambda:InvokeFunction",
        Resource: arn
      }
    ]
  }))
});

const scheduleRule = new aws.cloudwatch.EventRule("rss-feed-schedule", {
  description: "Trigger RSS feed check every hour (except 00:00-07:00 KST)",
  scheduleExpression: "cron(0 23,0-14 * * ? *)"
});

new aws.lambda.Permission("allow-eventbridge", {
  statementId: "AllowExecutionFromEventBridge",
  action: "lambda:InvokeFunction",
  function: feednyangRssFeedFunc.name,
  principal: "events.amazonaws.com",
  sourceArn: scheduleRule.arn
});

new aws.cloudwatch.EventTarget("lambda-target", {
  rule: scheduleRule.name,
  arn: feednyangRssFeedFunc.arn
});

export const feednyangRssFeedArn = feednyangRssFeedFunc.arn;
export const scheduleRuleArn = scheduleRule.arn;
