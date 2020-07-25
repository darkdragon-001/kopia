#!/bin/bash

# TODO GIT_EMAIL, GIT_NAME, GIT_USER, GIT_PASSWORD

# Setup git
git config --global user.email "${GIT_EMAIL}"
git config --global user.name "${GIT_NAME}"
git config --global push.default simple
git config credential.helper '!f() { sleep 1; echo "username=${GIT_USER}"; echo "password=${GIT_PASSWORD}"; }; f'
git config --global hub.protocol https

# Perform clone, commit, push
# NOTE using https://hub.github.com/  # TODO install
hub clone https://github.com/flathub/io.kopia.KopiaUI.git
cd io.kopia.KopiaUI
git checkout -b ${KOPIA_VERSION}
python3 ../tools/flathub.py "${KOPIA_VERSION}"  # TODO path to python script
git commit -am "Release ${KOPIA_VERSION}."
git tag -a -m "Release ${KOPIA_VERSION}." ${KOPIA_VERSION}
hub fork --remote-name ${GIT_USER}
git push ${GIT_USER} ${KOPIA_VERSION}
hub pull-request

