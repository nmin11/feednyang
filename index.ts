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

const weekdayScheduleRule = new aws.cloudwatch.EventRule("rss-feed-weekday-schedule", {
  description: "Trigger RSS feed check on weekdays at 08,12,18,22 KST",
  scheduleExpression: "cron(0 23,3,9,13 ? * MON-FRI *)"
});

const saturdayScheduleRule = new aws.cloudwatch.EventRule("rss-feed-saturday-schedule", {
  description: "Trigger RSS feed check on Saturday at 12 KST",
  scheduleExpression: "cron(0 3 ? * SAT *)"
});

new aws.lambda.Permission("allow-eventbridge-weekday", {
  statementId: "AllowExecutionFromEventBridgeWeekday",
  action: "lambda:InvokeFunction",
  function: feednyangRssFeedFunc.name,
  principal: "events.amazonaws.com",
  sourceArn: weekdayScheduleRule.arn
});

new aws.lambda.Permission("allow-eventbridge-saturday", {
  statementId: "AllowExecutionFromEventBridgeSaturday",
  action: "lambda:InvokeFunction",
  function: feednyangRssFeedFunc.name,
  principal: "events.amazonaws.com",
  sourceArn: saturdayScheduleRule.arn
});

new aws.cloudwatch.EventTarget("lambda-target-weekday", {
  rule: weekdayScheduleRule.name,
  arn: feednyangRssFeedFunc.arn
});

new aws.cloudwatch.EventTarget("lambda-target-saturday", {
  rule: saturdayScheduleRule.name,
  arn: feednyangRssFeedFunc.arn
});

export const feednyangRssFeedArn = feednyangRssFeedFunc.arn;
export const weekdayScheduleRuleArn = weekdayScheduleRule.arn;
export const saturdayScheduleRuleArn = saturdayScheduleRule.arn;
