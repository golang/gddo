$(function() {
    var prevCh = null, prevTime = 0, modal = false;

    $('.modal').on({
        show: function() { modal = true; },
        hidden: function() { modal = false; }
    });

    $(document).on('keypress', function(e) {
        var combo = e.timeStamp - prevTime <= 1000;
        prevTime = 0;

        if (modal) {
            return true;
        }

        var t = e.target.tagName
        if (t == 'INPUT' ||
            t == 'SELECT' ||
            t == 'TEXTAREA' ) {
            return true;
        }

        if (e.target.contentEditable && e.target.contentEditable == 'true') {
            return true;
        }

        var ch = String.fromCharCode(e.which);

        if (combo) {
            switch (prevCh + ch) {
            case "gg":
                $('html,body').animate({scrollTop: 0},'fast');
                return false;
            case "gb":
                $('html,body').animate({scrollTop: $(document).height()},'fast');
                return false;
            case "gi":
                if ($('#pkg-index').length > 0) {
                    $('html,body').animate({scrollTop: $("#pkg-index").offset().top},'fast');
                    return false;
                }
            case "ge":
                if ($('#pkg-examples').length > 0) {
                    $('html,body').animate({scrollTop: $("#pkg-examples").offset().top},'fast');
                    return false;
                }
            }
        }

        switch (ch) {
        case "/":
            $('#x-search-query').focus();
            return false;
        case "?":
            $('#x-shortcuts').modal();
            return false;
        case  ".":
            if ($('#x-jump').length > 0) {
                window.setTimeout(function() {
                    $('#x-jump-text').typeahead({local: symbols()});
                    $('#x-jump').modal();
                }, 0);
                return false;
            }
        }

        prevCh = ch
        prevTime = e.timeStamp
        return true;
    });

    $('span.timeago').timeago();
    if (window.location.hash.substring(0, 9) == '#example-') {
        var id = '#ex-' + window.location.hash.substring(9);
        console.log(id);
        console.log($(id));
       $(id).addClass('in').removeClass('collapse').height('auto');
    }

    var highlighted;

    function highlightHash() {
        if (highlighted) {
            highlighted.removeClass("highlight");
        }
        if (window.location.hash) {
            highlighted = $('#x-file ' + window.location.hash.replace(/([.])/g,'\\$1'));
            highlighted.addClass("highlight");
        } else {
            highlighted = null;
        }
    }
    window.onhashchange = highlightHash;
    highlightHash();

    $(document).on("click", "input.click-select", function(e) {
        $(e.target).select();
    });

});
