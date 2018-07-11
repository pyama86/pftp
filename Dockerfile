FROM fauria/vsftpd
ENV FTP_USER **String**
ENV FTP_PASS **Random**
ENV PASV_ADDRESS **IPv4**
ENV PASV_MIN_PORT 21100
ENV PASV_MAX_PORT 21110
ENV LOG_STDOUT **Boolean**
VOLUME /home/vsftpd
VOLUME /var/log/vsftpd
RUN echo "log_ftp_protocol=YES" >> /etc/vsftpd/vsftpd.conf
EXPOSE 20 21

CMD ["/usr/sbin/run-vsftpd.sh"]
