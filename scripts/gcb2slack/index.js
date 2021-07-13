const {
    IncomingWebhook
} = require('@slack/webhook');

const {
    auth
} = require('google-auth-library');

const webhook = new IncomingWebhook(process.env.SLACK_WEBHOOK_URL);

// Add additional statues to list if you'd like:
// QUEUED, WORKING, CANCELLED
const interestingStatuses = ['SUCCESS', 'FAILURE', 'INTERNAL_ERROR', 'TIMEOUT'];

// cloudbuildToSlack reads from a Google Cloud Build Pub/Sub topic and writes
// to a Slack webhook.
module.exports.cloudbuildToSlack = async (pubSubEvent, context) => {
    if (!pubSubEvent.data) {
        console.info("No `data` field in pubSubEvent");
        return;
    }
    const build = eventToBuild(pubSubEvent.data);

    // Skip if the current status is not in the status list.
    if (interestingStatuses.indexOf(build.status) === -1) {
        console.log("Build status %s is ignored", build.status);
        return;
    }

    // Send message to Slack.
    const message = await createSlackMessage(build);
    await webhook.send(message);
};

// eventToBuild transforms pubsub event message to a build object.
const eventToBuild = (data) => {
    return JSON.parse(Buffer.from(data, 'base64').toString());
}

const getTrigger = async (triggerId) => {
    const client = await auth.getClient({
        scopes: 'https://www.googleapis.com/auth/cloud-platform',
    });
    const projectId = await auth.getProjectId();
    const res = await client.request({
        url: `https://cloudbuild.googleapis.com/v1/projects/${projectId}/triggers/${triggerId}`,
    });
    return res.data;
}

// createSlackMessage creates a message from a build object.
const createSlackMessage = async (build) => {
    const buildStatus = build.statusDetail || build.status;
    const trigger = await getTrigger(build.buildTriggerId);

    let {
        REPO_NAME: repo,
        BRANCH_NAME: branch,
    } = build.substitutions;

    // If there was no substitution containing the repository name,
    // take it from build.source. Unfortunately, this name will be the name
    // used by Google Cloud Source Repositories, not the name on GitHub.
    if (!repo && build.source.repoSource) {
        repo = build.source.repoSource.repoName;
    }

    let messagePrefix, color;
    switch (build.status) {
        case 'TIMEOUT':
            messagePrefix = "Timeout of ";
            color = 'danger';
            break;
        case 'FAILURE':
            messagePrefix = "Failed to ";
            color = 'danger';
            break;
        case 'INTERNAL_ERROR':
            messagePrefix = "Internal error while trying to ";
            color = 'warning';
            break;
        case 'SUCCESS':
            messagePrefix = "Completed ";
            color = 'good';
            break;
    }

    let message = {
        text: `${messagePrefix}'${trigger.description}' for ${repo} repo`,
        attachments: [{
            fallback: buildStatus,
            color: color,
            fields: [{
                title: 'Status',
                value: buildStatus,
                short: true,
            }, {
                title: 'Build Logs',
                value: build.logUrl,
            }],
        }],
    };

    if (branch) {
        message.attachments[0].fields.push({
            title: 'Branch',
            value: branch,
        })
    }

    return message
}
