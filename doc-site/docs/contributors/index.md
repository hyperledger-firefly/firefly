---
title: Contributors' Guide
---

We welcome anyone to contribute to the FireFly project! If you're interested, this is a guide on how to get started. **You don't have to be a blockchain expert to make valuable contributions!** There are lots of places for developers of all experience levels to get involved.

🧑🏽‍💻 👩🏻‍💻 👩🏾‍💻 🧑🏻‍💻 🧑🏿‍💻 👨🏽‍💻 👩🏽‍💻 🧑🏾‍💻 👨🏿‍💻 👨🏾‍💻 👩🏿‍💻 👨🏻‍💻

---

## 🚀 Connect with us on Discord

You can chat with maintainers and other contributors on Discord in the `firefly` channel:
[https://discord.gg/hyperledger](https://discord.gg/hyperledger)

[Join Discord Server](https://discord.gg/hyperledger){: .md-button .md-button--primary}

## 📅 Join our Community Calls

Community calls are a place to talk to other contributors, maintainers, and other people interested in FireFly. Maintainers often discuss upcoming changes and proposed new features on these calls. These calls are a great way for the community to give feedback on new ideas, ask questions about FireFly, and hear how others are using FireFly to solve real world problems.

Please see the [FireFly Calendar](https://lists.hyperledger.org/g/firefly/calendar) for the current meeting schedule, and the link to join. Everyone is welcome to join, regardless of background or experience level.

## 🔍 Find your first issue

If you're looking for somewhere to get started in the FireFly project and want something small and relatively easy, take a look at [issues tagged with "Good first issue"](https://github.com/search?q=repo%3Ahyperledger%2Ffirefly+repo%3Ahyperledger%2Ffirefly-ethconnect+repo%3Ahyperledger%2Ffirefly-evmconnect+repo%3Ahyperledger%2Ffirefly-fabconnect+repo%3Ahyperledger%2Ffirefly-cordaconnect+repo%3Ahyperledger%2Ffirefly-transaction-manager+repo%3Ahyperledger%2Ffirefly-cli+repo%3Ahyperledger%2Ffirefly-samples+repo%3Ahyperledger%2Ffirefly-dataexchange-https+repo%3Ahyperledger%2Ffirefly-ui+repo%3Ahyperledger%2Ffirefly-helm-charts+repo%3Ahyperledger%2Ffirefly-sandbox+repo%3Ahyperledger%2Ffirefly-signer+repo%3Ahyperledger%2Ffirefly-common+repo%3Ahyperledger%2Ffirefly-perf-cli+repo%3Ahyperledger%2Ffirefly-tokens-erc1155+repo%3Ahyperledger%2Ffirefly-tokens-erc20-erc721+repo%3Ahyperledger%2Ffirefly-sdk-nodejs+label%3A%22Good+first+issue%22+state%3Aopen&type=Issues&ref=advsearch&l=&l=). You can definitely work on other things if you want to. These are only suggestions for easy places to get started.

[See "Good First Issues"](https://github.com/search?q=repo%3Ahyperledger%2Ffirefly+repo%3Ahyperledger%2Ffirefly-ethconnect+repo%3Ahyperledger%2Ffirefly-evmconnect+repo%3Ahyperledger%2Ffirefly-fabconnect+repo%3Ahyperledger%2Ffirefly-cordaconnect+repo%3Ahyperledger%2Ffirefly-transaction-manager+repo%3Ahyperledger%2Ffirefly-cli+repo%3Ahyperledger%2Ffirefly-samples+repo%3Ahyperledger%2Ffirefly-dataexchange-https+repo%3Ahyperledger%2Ffirefly-ui+repo%3Ahyperledger%2Ffirefly-helm-charts+repo%3Ahyperledger%2Ffirefly-sandbox+repo%3Ahyperledger%2Ffirefly-signer+repo%3Ahyperledger%2Ffirefly-common+repo%3Ahyperledger%2Ffirefly-perf-cli+repo%3Ahyperledger%2Ffirefly-tokens-erc1155+repo%3Ahyperledger%2Ffirefly-tokens-erc20-erc721+repo%3Ahyperledger%2Ffirefly-sdk-nodejs+label%3A%22Good+first+issue%22+state%3Aopen&type=Issues&ref=advsearch&l=&l=){: .md-button .md-button--primary}

> **NOTE** Hyperledger FireFly has a microservice architecture so it has many different GitHub repos. Use the link or the button above to look for "Good First Issues" across all the repos at once.

Here are some other suggestions of places to get started, based on experience you may already have:

### Any level of experience

If you looking to make your first open source contribution the [FireFly documentation](https://github.com/hyperledger-firefly/firefly/tree/main/docs) is a great place to make small, easy improvements. These improvements are also very valuable, because they help the next person that may want to know the same thing.

Here are some detailed instructions on [Contributing to Documentation](./docs_setup.md)

### Go experience

If you have some experience in Go and really want to jump into FireFly, the [FireFly Core](https://github.com/hyperledger-firefly/firefly/issues) is the heart of the project.

Here are some detailed instructions on [Setting up a FireFly Core Development Environment](./dev_environment_setup.md).

### Little or no Go experience, but want to learn

If you don't have a lot of experience with Go, but are interested in learning, the [FireFly CLI](https://github.com/hyperledger-firefly/cli/issues) might be a good place to start. The FireFly CLI is a tool to set up local instances of FireFly for building apps that use FireFly, and for doing development on FireFly itself.

### TypeScript experience

If you have some experience in TypeScript, there are several FireFly microservices that are written in TypeScript. The [Data Exchange](https://github.com/hyperledger-firefly/dataexchange-https/issues) is used for private messaging between FireFly nodes. The [ERC-20/ERC-271 Tokens Connector](https://github.com/hyperledger-firefly/tokens-erc20-erc721/issues) and [ERC-1155 Tokens Connector](https://github.com/hyperledger-firefly/tokens-erc1155/issues) are used to abstract token contract specifics from the FireFly Core.

### React/TypeScript experience

If you want to do some frontend development, the [FireFly UI](https://github.com/hyperledger-firefly/ui/issues) is written in TypeScript and React.

### Go and blockchain experience

If you already have some experience with blockchain and want to work on some backend components, the blockchain connectors, [firefly-ethconnect](https://github.com/hyperledger-firefly/ethconnect/issues) (for Ethereum) and [firefly-fabconnect](https://github.com/hyperledger-firefly/fabconnect/issues) (for Fabric) are great places to get involved.

## 📝 Make changes

To contribute to the repository, please [fork the repository](https://docs.github.com/en/get-started/quickstart/fork-a-repo) that you want to change. Then clone your fork locally on your machine and make your changes. As you commit your changes, push them to your fork. More information on making commits below.

## 📑 Commit with Developer Certificate of Origin

As with all Hyperledger repositories, FireFly requires proper sign-off on every commit that is merged into the `main` branch. The sign-off indicates that you certify the changes you are submitting are in accordance with the [Developer Certificate of Origin](https://developercertificate.org/). To sign-off on your commit, you can use the `-s` flag when you commit changes.

```
git commit -s -m "Your commit message"
```

This will add a string like this to the end of your commit message:

```
"Signed-off-by: Your Name <your-email@address>"
```

> **NOTE:** [Sign-off](https://git-scm.com/docs/git-commit#Documentation/git-commit.txt--s) is _not_ the same thing as [signing your commits](https://git-scm.com/docs/git-commit#Documentation/git-commit.txt--Sltkeyidgt) with a private key. Both operations use a similar flag, which can be confusing. The one you want is the _lowercase_ `-s` 🙂

## 📥 Open a Pull Request

When you're ready to submit your changes for review, [open a Pull Request back to the upstream repository](https://docs.github.com/en/github/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request-from-a-fork). When you open your pull request, the maintainers will automatically be notified. Additionally, a series of automated checks will be performed on your code to make sure it passes certain repository specific requirements.

Maintainers may have suggestions on things to improve in your pull request. It is our goal to get code that is beneficial to the project merged as quickly as possible, so we don't like to leave pull requests hanging around for a long time. If the project maintainers are satisfied with the changes, they will approve and merge the pull request.

Thanks for your interest in collaborating on this project!

## Inclusivity

The Hyperledger Foundation and the FireFly project are committed to fostering a community that is welcoming to all people. When participating in community discussions, contributing code, or documentaiton, please abide by the following guidelines:

- Consider that users who will read the docs are from different background and cultures and that they have different preferences.
- Avoid potential offensive terms and, for instance, prefer "allow list and deny list" to "white list and black list".
- We believe that we all have a role to play to improve our world, and even if writing inclusive doc might not look like a huge improvement, it's a first step in the right direction.
- We suggest to refer to Microsoft bias free writing guidelines and Google inclusive doc writing guide as starting points.
