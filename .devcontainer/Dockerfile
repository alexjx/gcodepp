# hub.fastonetech.com/tools/docker/dockerfile-upstream:1.4.3
FROM hub.fastonetech.com/tools/golang-node:1.21.0-20.5.1

ARG USERNAME
ARG USER_UID=1000
ARG USER_GID=$USER_UID


RUN apt update && \
    apt install -y sudo vim graphviz bash-completion git && \
    ln -s /usr/share/bash-completion/completions/git /etc/bash_completion.d/git


RUN if [ "$USERNAME" != "root" ]; then \
        groupadd --gid $USER_GID $USERNAME && \
        useradd --uid $USER_UID --gid $USER_GID -m $USERNAME -s /bin/bash && \
        mkdir -p /home/$USERNAME/.vscode-server/extensions \
                /home/$USERNAME/.vscode-server-insiders/extensions \
                /home/$USERNAME/.cache \
                /home/$USERNAME/.local \
        && chown -R $USERNAME \
            /home/$USERNAME/.vscode-server \
            /home/$USERNAME/.vscode-server-insiders \
            /home/$USERNAME/.cache \
            /home/$USERNAME/.local && \
        echo "$USERNAME ALL=\(ALL\) NOPASSWD:ALL" > /etc/sudoers.d/$USERNAME && \
        chmod 0440 /etc/sudoers.d/$USERNAME || exit 1; \
    fi

RUN curl -o /usr/local/bin/buf http://mirrors.fastonetech.com/buf-1.26.1 && \
    chmod +x /usr/local/bin/buf

USER $USERNAME
