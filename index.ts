import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";

const lambdaRole = new aws.iam.Role("discord-lambda-role", {
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

new aws.iam.RolePolicyAttachment("discord-lambda-policy", {
  role: lambdaRole.name,
  policyArn: "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
});

const discordLambdaFunc = new aws.lambda.Function("discord-rss-feed", {
  code: new pulumi.asset.AssetArchive({
    ".": new pulumi.asset.FileArchive("./lambda/discord-rss-feed"),
  }),
  runtime: "provided.al2023",
  handler: "bootstrap",
  role: lambdaRole.arn,
  environment: {
    variables: {
      DISCORD_WEBHOOK_URL: new pulumi.Config().require("discord-webhook-url"),
    }
  },
  timeout: 30
});

export const discordLambdaArn = discordLambdaFunc.arn;
